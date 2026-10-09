package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type releasePackage struct {
	updateArtifact
	ID           string            `json:"id"`
	Version      string            `json:"version"`
	DisplayName  string            `json:"display_name"`
	Label        map[string]string `json:"label,omitempty"`
	Desc         map[string]string `json:"desc,omitempty"`
	Kind         string            `json:"kind"`
	Target       string            `json:"target"`
	Core         spec.VersionRange `json:"core"`
	Platforms    []string          `json:"platforms"`
	Environments []string          `json:"environments,omitempty"`
	Permissions  []string          `json:"permissions"`
}

func (item releasePackage) valid(repository, tag string) bool {
	suffix := path.Ext(item.Name)
	if !spec.ValidID(item.ID) || !stableVersion.MatchString(item.Version) || item.Core.Validate() != nil || spec.ValidateEnvironments(item.Environments) != nil {
		return false
	}
	if !spec.SafePath(item.Name) || path.Base(item.Name) != item.Name {
		return false
	}
	if !((item.Target == "backend" && (suffix == ".cphext" || suffix == ".cphhost")) ||
		(item.Target == "client" && item.Kind == "frontend-trusted" && suffix == ".cphui")) {
		return false
	}
	return item.DownloadURL == repository+"/releases/download/"+tag+"/"+url.PathEscape(item.Name) &&
		updateHash.MatchString(item.SHA256) && item.Size > 0 && item.Size <= extension.MaxPackageBytes
}

func (item releasePackage) onThisPlatform() bool {
	if len(item.Platforms) == 0 {
		return true
	}
	for _, platform := range item.Platforms {
		if platform == runtime.GOOS+"/"+runtime.GOARCH {
			return true
		}
	}
	return false
}

type cachedExtensionCatalog struct {
	URL       string         `json:"url"`
	CheckedAt time.Time      `json:"checked_at"`
	Manifest  remoteManifest `json:"manifest"`
}

func (s *Server) extensionIndex(ctx context.Context, refresh bool) (cachedExtensionCatalog, bool, error) {
	var cached cachedExtensionCatalog
	validCache := json.Unmarshal([]byte(s.settings.Get(setting.KeyExtensionCatalog, "")), &cached) == nil &&
		cached.URL == version.UpdateManifestURL() && cached.Manifest.valid(version.UpdateRepository)
	if refresh || !validCache {
		if manifest := s.fetchLatestVersion(ctx); manifest != nil {
			next := cachedExtensionCatalog{URL: version.UpdateManifestURL(), CheckedAt: time.Now().UTC(), Manifest: *manifest}
			data, err := json.Marshal(next)
			if err == nil {
				err = s.settings.Set(setting.KeyExtensionCatalog, string(data))
			}
			return next, false, err
		}
	}
	if validCache {
		return cached, true, nil
	}
	return cachedExtensionCatalog{}, false, fmt.Errorf("release package index is unavailable; mounted packages remain available offline")
}

func (s *Server) extensionMarketplace(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	index, cached, err := s.extensionIndex(r.Context(), r.URL.Query().Get("refresh") == "true")
	if err != nil {
		extensionReply(w, err)
		return
	}
	type entry struct {
		releasePackage
		Status           string `json:"status"`
		InstalledVersion string `json:"installed_version,omitempty"`
	}
	items := []entry{}
	for _, item := range index.Manifest.Packages {
		if item.Target != "backend" || !item.onThisPlatform() || s.extensions.CheckManagement(r.Context(), spec.Manifest{ID: item.ID, Kind: item.Kind}) != nil {
			continue
		}
		value := entry{releasePackage: item, Status: "available"}
		if !item.Core.Accepts(version.Core) {
			value.Status = "incompatible"
		} else if installed, ok := s.extensions.State(item.ID); ok {
			value.InstalledVersion = installed.Manifest.Version
			comparison, _ := spec.Compare(item.Version, installed.Manifest.Version)
			switch {
			case installed.Hash == item.SHA256:
				value.Status = "installed"
			case comparison >= 0:
				value.Status = "update"
			default:
				value.Status = "older"
			}
		}
		items = append(items, value)
	}
	writeJSON(w, 200, map[string]any{"packages": items, "cached": cached, "checked_at": index.CheckedAt, "release_url": index.Manifest.ReleaseURL})
}

// 在线目录只用于选择下载；权限确认展示重新验签后的清单，安装使用短期票据。
func (s *Server) inspectMarketExtension(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var input struct {
		SHA256 string `json:"sha256"`
	}
	if !readBody(w, r, &input) {
		return
	}
	index, _, err := s.extensionIndex(r.Context(), false)
	if err != nil {
		extensionReply(w, err)
		return
	}
	var selected *releasePackage
	for _, item := range index.Manifest.Packages {
		if item.Target == "backend" && item.SHA256 == input.SHA256 && item.onThisPlatform() && item.Core.Accepts(version.Core) && s.extensions.CheckManagement(r.Context(), spec.Manifest{ID: item.ID, Kind: item.Kind}) == nil {
			copy := item
			selected = &copy
			break
		}
	}
	if selected == nil {
		extensionReply(w, fmt.Errorf("no compatible package in the release index; refresh the index"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.withGitHubProxy(selected.DownloadURL), nil)
	if err != nil {
		extensionReply(w, err)
		return
	}
	response, err := downloadHTTPClient.Do(req)
	if err != nil {
		extensionReply(w, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		extensionReply(w, fmt.Errorf("package download returned HTTP %d", response.StatusCode))
		return
	}
	file, err := os.CreateTemp(s.dataDir, ".extension-download-")
	if err != nil {
		extensionReply(w, err)
		return
	}
	defer os.Remove(file.Name())
	size, err := io.Copy(file, io.LimitReader(response.Body, selected.Size+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || size != selected.Size {
		extensionReply(w, fmt.Errorf("incomplete or oversized package download"))
		return
	}
	verified, err := s.extensions.Inspect(file.Name())
	if err == nil {
		err = s.extensions.CheckManagement(ctx, verified.Manifest)
	}
	if err == nil && (verified.SHA256 != selected.SHA256 || verified.Manifest.ID != selected.ID || verified.Manifest.Version != selected.Version) {
		err = fmt.Errorf("downloaded package differs from the release index")
	}
	if err != nil {
		extensionReply(w, err)
		return
	}
	ticket, err := s.extensions.StageUpload(file.Name())
	if err != nil {
		extensionReply(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"manifest": verified.Manifest, "hash": verified.SHA256, "signer": verified.Signature.KeyID,
		"publisher": verified.Publisher, "ticket": ticket})
}
