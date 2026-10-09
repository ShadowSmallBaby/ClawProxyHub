package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

// CatalogEntry 来自挂载包的已验证清单；无效包只暴露路径和错误。
type CatalogEntry struct {
	Path             string         `json:"path"`
	SHA256           string         `json:"sha256,omitempty"`
	Manifest         *spec.Manifest `json:"manifest,omitempty"`
	Signer           string         `json:"signer,omitempty"`
	Publisher        string         `json:"publisher,omitempty"`
	Status           string         `json:"status"`
	Error            string         `json:"error,omitempty"`
	InstalledVersion string         `json:"installed_version,omitempty"`
	InstalledHash    string         `json:"installed_hash,omitempty"`
	Enabled          bool           `json:"enabled"`
	Available        bool           `json:"available"`
}

// SetPackageDirs 仅配置输入目录，安装文件始终写入管理器自己的数据目录。
func (m *Manager) SetPackageDirs(dirs []string) {
	m.sourcesMu.Lock()
	defer m.sourcesMu.Unlock()
	m.packageDirs = append([]string{}, dirs...)
}

func (m *Manager) receipt(id string) string { return filepath.Join(m.dir, "receipts", id+".json") }

// remember 保存已处理身份，卸载后不会因包仍挂载而重新安装。
func (m *Manager) remember(id string) error {
	if !spec.ValidID(id) {
		return fmt.Errorf("invalid package identity")
	}
	path := m.receipt(id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(map[string]string{"id": id})
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func (m *Manager) remembered(id string) (bool, error) {
	_, err := os.Stat(m.receipt(id))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (m *Manager) scanPackages() []CatalogEntry {
	out := []CatalogEntry{}
	seen := map[string]bool{}
	for _, dir := range m.packageDirs {
		absolute, err := filepath.Abs(dir)
		if err != nil {
			out = append(out, CatalogEntry{Path: dir, Status: "invalid", Error: err.Error()})
			continue
		}
		entries, err := os.ReadDir(absolute)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			out = append(out, CatalogEntry{Path: absolute, Status: "invalid", Error: err.Error()})
			continue
		}
		for _, entry := range entries {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".cphext" && ext != ".cphhost" {
				continue
			}
			path := filepath.Join(absolute, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			item := CatalogEntry{Path: path, Status: "invalid"}
			if !entry.Type().IsRegular() {
				item.Error = "package must be a regular file"
				out = append(out, item)
				continue
			}
			v, err := m.Inspect(path)
			if err != nil {
				item.Error = err.Error()
				out = append(out, item)
				continue
			}
			item.Manifest, item.SHA256, item.Signer, item.Publisher = &v.Manifest, v.SHA256, v.Signature.KeyID, v.Publisher
			item.Status = "available"
			if ext == ".cphhost" && v.Manifest.Kind != "runtime" {
				item.Status, item.Error = "invalid", "host package must contain a runtime"
			} else if v.Manifest.Target != "backend" || !v.Manifest.Core.Accepts(m.core) || (len(v.Manifest.Platforms) > 0 && !contains(v.Manifest.Platforms, runtime.GOOS+"/"+runtime.GOARCH)) {
				item.Status, item.Error = "incompatible", "package does not support this core or platform"
			} else if s, ok := m.State(v.Manifest.ID); ok {
				item.InstalledVersion, item.InstalledHash = s.Manifest.Version, s.Hash
				item.Enabled, item.Available = s.Enabled, s.Available
				comparison, _ := spec.Compare(v.Manifest.Version, s.Manifest.Version)
				switch {
				case s.Hash == v.SHA256:
					item.Status = "installed"
				case comparison >= 0:
					item.Status = "update"
				default:
					item.Status = "older"
				}
			} else if yes, err := m.remembered(v.Manifest.ID); err != nil {
				item.Status, item.Error = "invalid", err.Error()
			} else if yes {
				item.Status = "removed"
			}
			out = append(out, item)
		}
	}
	// 同一平台的同版本包不能靠文件名顺序决定安装内容。
	identities := map[string]string{}
	conflicts := map[string]bool{}
	for _, item := range out {
		if item.Manifest == nil || item.Status == "invalid" || item.Status == "incompatible" {
			continue
		}
		key := item.Manifest.ID + "@" + item.Manifest.Version
		if old := identities[key]; old != "" && old != item.SHA256 {
			conflicts[key] = true
		}
		identities[key] = item.SHA256
	}
	for i := range out {
		if out[i].Manifest != nil && out[i].Status != "incompatible" && conflicts[out[i].Manifest.ID+"@"+out[i].Manifest.Version] {
			out[i].Status, out[i].Error = "conflict", "mounted packages disagree on the same version"
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (m *Manager) Catalog() []CatalogEntry {
	m.sourcesMu.Lock()
	defer m.sourcesMu.Unlock()
	out := m.scanPackages()
	for i := range out {
		if err := m.importErrors[out[i].SHA256]; err != "" && out[i].Status == "available" {
			out[i].Status, out[i].Error = "blocked", err
		}
	}
	return out
}

// ImportMounted 在插件启动前安装未处理的可信包；已有安装和卸载收据均跳过。
func (m *Manager) ImportMounted(ctx context.Context) error {
	m.sourcesMu.Lock()
	defer m.sourcesMu.Unlock()
	m.importErrors = map[string]string{}
	selected := map[string]CatalogEntry{}
	for _, item := range m.scanPackages() {
		if item.Status != "available" {
			continue
		}
		id := item.Manifest.ID
		old, exists := selected[id]
		if c, _ := spec.Compare(item.Manifest.Version, versionOf(old)); !exists || c > 0 {
			selected[id] = item
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for len(selected) > 0 {
		progress := false
		for _, id := range ids {
			item, ok := selected[id]
			if !ok {
				continue
			}
			if _, installed := m.State(id); installed {
				delete(selected, id)
				progress = true
				continue
			}
			if err := m.dependencies(*item.Manifest); err != nil {
				m.importErrors[item.SHA256] = err.Error()
				continue
			}
			delete(selected, id)
			progress = true
			if _, err := m.install(ctx, item.Path, item.Manifest.Permissions, item.SHA256, true); err != nil {
				m.importErrors[item.SHA256] = err.Error()
				continue
			}
			delete(m.importErrors, item.SHA256)
			if err := m.remember(id); err != nil {
				return err
			}
		}
		if !progress {
			break
		}
	}
	return nil
}

func versionOf(item CatalogEntry) string {
	if item.Manifest == nil {
		return "0.0.0"
	}
	return item.Manifest.Version
}

// InstallMounted 用清单中的摘要重新校验输入，客户端不能指定任意服务器路径。
func (m *Manager) InstallMounted(ctx context.Context, digest string, grants []string) (State, error) {
	if !spec.ValidHash(digest) {
		return State{}, fmt.Errorf("invalid package digest")
	}
	for _, item := range m.Catalog() {
		if item.SHA256 != digest {
			continue
		}
		if item.Status == "invalid" || item.Status == "incompatible" || item.Status == "conflict" || item.Status == "older" {
			return State{}, fmt.Errorf("package is %s: %s", item.Status, item.Error)
		}
		return m.InstallExpected(ctx, item.Path, grants, digest)
	}
	return State{}, fmt.Errorf("mounted package no longer exists; refresh the catalog")
}
