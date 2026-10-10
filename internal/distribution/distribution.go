// Package distribution 装配内嵌 Web 的核心、CLI 与签名包。
package distribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type Request struct {
	Profile  string          `json:"profile"`
	Core     string          `json:"core"`
	CLI      string          `json:"cli"`
	Version  string          `json:"version"`
	Platform string          `json:"platform"`
	Frontend json.RawMessage `json:"frontend"`
	Readme   string          `json:"readme,omitempty"`
	License  string          `json:"license,omitempty"`
	Packages []string        `json:"packages"`
	Trust    spec.TrustStore `json:"trust"`
}
type Package struct {
	ID           string                       `json:"id"`
	Version      string                       `json:"version"`
	Path         string                       `json:"path"`
	SHA256       string                       `json:"sha256"`
	Signer       string                       `json:"signer"`
	Grants       []string                     `json:"grants"`
	Dependencies map[string]spec.VersionRange `json:"dependencies,omitempty"`
}
type Lock struct {
	Format     int               `json:"format"`
	Profile    string            `json:"profile"`
	Version    string            `json:"version"`
	Platform   string            `json:"platform"`
	Core       string            `json:"core"`
	CLI        string            `json:"cli"`
	CoreSHA256 string            `json:"core_sha256"`
	Frontend   json.RawMessage   `json:"frontend"`
	Builtins   []string          `json:"builtins"`
	Files      map[string]string `json:"files"`
	Packages   []Package         `json:"packages"`
	Trust      spec.TrustStore   `json:"trust"`
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func fileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func validProfile(p string) bool { return p == "full" }
func Assemble(r Request, out string) (Lock, error) {
	l := Lock{Format: 1, Profile: r.Profile, Version: r.Version, Platform: r.Platform, Frontend: r.Frontend, Files: map[string]string{}, Trust: r.Trust, Packages: []Package{}, Builtins: []string{"storage", "auth", "extensions", "plugins", "accounts", "tasks"}}
	if !validProfile(r.Profile) {
		return l, fmt.Errorf("unknown profile")
	}
	if _, e := spec.Version(r.Version); e != nil {
		return l, e
	}
	platform := strings.Split(r.Platform, "/")
	if len(platform) != 2 || !spec.ValidID(platform[0]) || !spec.ValidID(platform[1]) {
		return l, fmt.Errorf("invalid platform")
	}
	l.Builtins = append(l.Builtins, "gateway")
	if r.CLI == "" || !json.Valid(r.Frontend) {
		return l, fmt.Errorf("full distribution requires CLI and embedded frontend metadata")
	}
	if _, e := os.Lstat(out); !errors.Is(e, fs.ErrNotExist) {
		return l, fmt.Errorf("output must not exist")
	}
	if e := os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		return l, e
	}
	stage, e := os.MkdirTemp(filepath.Dir(out), ".assemble-")
	if e != nil {
		return l, e
	}
	defer os.RemoveAll(stage)
	write := func(name string, b []byte, mode fs.FileMode, immutable bool) error {
		if e := os.MkdirAll(filepath.Dir(filepath.Join(stage, name)), 0755); e != nil {
			return e
		}
		if e := os.WriteFile(filepath.Join(stage, name), b, mode); e != nil {
			return e
		}
		if immutable {
			l.Files[name] = hash(b)
		}
		return nil
	}
	core, e := os.ReadFile(r.Core)
	if e != nil {
		return l, e
	}
	l.Core = "cph"
	l.CLI = "cli"
	if platform[0] == "windows" {
		l.Core += ".exe"
		l.CLI += ".exe"
	}
	if e = write(l.Core, core, 0755, true); e != nil {
		return l, e
	}
	l.CoreSHA256 = hash(core)
	for _, item := range []struct {
		name, path string
		mode       fs.FileMode
	}{{l.CLI, r.CLI, 0755}, {"README.md", r.Readme, 0644}, {"LICENSE", r.License, 0644}} {
		if item.path == "" {
			continue
		}
		data, err := os.ReadFile(item.path)
		if err != nil {
			return l, err
		}
		if err = write(item.name, data, item.mode, true); err != nil {
			return l, err
		}
	}
	verified := map[string]*extension.Verified{}
	for _, path := range r.Packages {
		v, e := extension.Verify(path, r.Trust)
		if e != nil {
			return l, e
		}
		m := v.Manifest
		if verified[m.ID] != nil || m.Target != "backend" || !m.Core.Accepts(r.Version) {
			return l, fmt.Errorf("incompatible or duplicate package %s", m.ID)
		}
		if len(m.Platforms) > 0 {
			ok := false
			for _, p := range m.Platforms {
				ok = ok || p == r.Platform
			}
			if !ok {
				return l, fmt.Errorf("package platform mismatch: %s", m.ID)
			}
		}
		verified[m.ID] = v
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("dependency cycle: %s", id)
		}
		if done[id] {
			return nil
		}
		v := verified[id]
		if v == nil {
			return fmt.Errorf("missing dependency: %s", id)
		}
		visiting[id] = true
		keys := []string{}
		for dep := range v.Manifest.Dependencies {
			keys = append(keys, dep)
		}
		sort.Strings(keys)
		for _, dep := range keys {
			if e := visit(dep); e != nil {
				return e
			}
			if !v.Manifest.Dependencies[dep].Accepts(verified[dep].Manifest.Version) {
				return fmt.Errorf("incompatible dependency: %s", dep)
			}
		}
		ext := ".cphext"
		if v.Manifest.Kind == "runtime" {
			ext = ".cphhost"
		}
		name := "data/packages/" + id + "-" + v.Manifest.Version + ext
		if e := write(name, v.Archive, 0644, false); e != nil {
			return e
		}
		l.Packages = append(l.Packages, Package{id, v.Manifest.Version, name, v.SHA256, v.Signature.KeyID, v.Manifest.Permissions, v.Manifest.Dependencies})
		delete(visiting, id)
		done[id] = true
		return nil
	}
	ids := []string{}
	for id := range verified {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if e = visit(id); e != nil {
			return l, e
		}
	}
	b, e := json.MarshalIndent(l, "", "  ")
	if e != nil {
		return l, e
	}
	if e = os.WriteFile(filepath.Join(stage, "distribution.lock.json"), b, 0644); e != nil {
		return l, e
	}
	return l, os.Rename(stage, out)
}

type Bundle struct {
	Lock Lock
	Dir  string
}

// Open 校验发行程序；可增删的原始包在安装时分别验签。
func Open(dir, executable, version string) (*Bundle, error) {
	root, e := os.OpenRoot(dir)
	if e != nil {
		return nil, e
	}
	defer root.Close()
	b, e := root.ReadFile("distribution.lock.json")
	if e != nil {
		return nil, e
	}
	var l Lock
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&l); e != nil {
		return nil, e
	}
	if l.Format != 1 || !validProfile(l.Profile) || l.Version != version || l.Platform != runtime.GOOS+"/"+runtime.GOARCH || !spec.SafePath(l.Core) || !spec.SafePath(l.CLI) || !spec.ValidHash(l.Files[l.CLI]) || l.Files[l.Core] != l.CoreSHA256 || !json.Valid(l.Frontend) {
		return nil, fmt.Errorf("incompatible distribution lock")
	}
	actual, e := fileHash(executable)
	if e != nil {
		return nil, e
	}
	if actual != l.CoreSHA256 {
		return nil, fmt.Errorf("running core differs from distribution lock")
	}
	bundle := &Bundle{Lock: l, Dir: dir}
	for path, want := range l.Files {
		if !spec.SafePath(path) || !spec.ValidHash(want) {
			return nil, fmt.Errorf("unsafe distribution file")
		}
		f, e := root.Open(path)
		if e != nil {
			return nil, e
		}
		data, e := io.ReadAll(io.LimitReader(f, 256<<20+1))
		f.Close()
		if e != nil {
			return nil, e
		}
		if len(data) > 256<<20 || hash(data) != want {
			return nil, fmt.Errorf("distribution hash mismatch: %s", path)
		}
	}
	for _, p := range l.Packages {
		if !spec.ValidID(p.ID) || !spec.SafePath(p.Path) || !strings.HasPrefix(p.Path, "data/packages/") || !spec.ValidHash(p.SHA256) {
			return nil, fmt.Errorf("invalid locked package")
		}
	}
	return bundle, nil
}
func (b *Bundle) Profile() string {
	return b.Lock.Profile
}

// PackageDir 与用户挂载目录共用发现、验签和安装流程。
func (b *Bundle) PackageDir() string { return filepath.Join(b.Dir, "data", "packages") }
