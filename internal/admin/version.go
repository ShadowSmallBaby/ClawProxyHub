// version.go — 核心版本回显与检查更新。
package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
)

// remoteManifest 保留桌面顶层字段；其他客户端使用同一文件内的 platform 对象。
type remoteManifest struct {
	Schema     int                                  `json:"schema_version"`
	Tag        string                               `json:"tag"`
	Version    string                               `json:"version"`
	ReleaseURL string                               `json:"release_url"`
	Artifacts  map[string]map[string]updateArtifact `json:"artifacts"`
	Packages   []releasePackage                     `json:"packages,omitempty"`
	Changelog  struct {
		Title map[string]string   `json:"title"`
		Items []map[string]string `json:"items"`
	} `json:"changelog"`
}

type updateArtifact struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var updateHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// coreVersion GET /admin/version — 本机版本 + 远端最新版对比（远端不可达时静默降级，只回本机版本）。
// 有更新时附带远端更新日志（title/items 中英双语），供前端弹窗展示。
func (s *Server) coreVersion(w http.ResponseWriter, r *http.Request) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	out := map[string]interface{}{"version": version.Core, "target": "core", "platform": platform, "update_method": "replace-distribution", "release_url": version.LatestReleaseURL()}
	if runtime.GOOS == "android" {
		out["update_method"] = "android-app"
		out["update_available"] = false
		writeJSON(w, http.StatusOK, out)
		return
	}
	if m := s.fetchLatestVersion(r.Context()); m != nil {
		out["latest"] = m.Version
		// 仅当远端版本严格大于本机时才提示更新（避免本地领先/降级被误判为可更新）
		out["update_available"] = compareSemver(m.Version, version.Core) > 0 && len(m.Artifacts[platform]) > 0
		out["release_url"] = m.ReleaseURL
		if len(m.Changelog.Items) > 0 {
			out["changelog"] = m.Changelog
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// fetchLatestVersion 从 latest Release 读取统一清单，失败时只回显本地版本。
func (s *Server) fetchLatestVersion(ctx context.Context) *remoteManifest {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.withGitHubProxy(version.UpdateManifestURL()), nil)
	if err != nil {
		return nil
	}
	resp, err := marketHTTPClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var m remoteManifest
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 || json.Unmarshal(data, &m) != nil || !m.valid(version.UpdateRepository) {
		return nil
	}
	return &m
}

func (m *remoteManifest) valid(repository string) bool {
	if m.Schema != 1 || !stableVersion.MatchString(m.Version) || !stableVersion.MatchString(strings.TrimPrefix(m.Tag, "v")) || m.ReleaseURL != repository+"/releases/tag/"+m.Tag {
		return false
	}
	for platform, profiles := range m.Artifacts {
		for profile, artifact := range profiles {
			name := "cph-" + strings.ReplaceAll(platform, "/", "-") + "-" + profile + ".zip"
			if profile != "full" || artifact.Name != name || artifact.DownloadURL != repository+"/releases/download/"+m.Tag+"/"+name || !updateHash.MatchString(artifact.SHA256) || artifact.Size < 1 {
				return false
			}
		}
	}
	if len(m.Packages) > 2048 {
		return false
	}
	for _, item := range m.Packages {
		if !item.valid(repository, m.Tag) {
			return false
		}
	}
	return true
}

// compareSemver 逐段比较点分版本号（忽略 v 前缀与预发布后缀）：a>b 返回 1，a<b 返回 -1，相等返回 0。
func compareSemver(a, b string) int {
	pa := splitSemver(a)
	pb := splitSemver(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// splitSemver 取版本主体的数值段（如 "v1.0.5-beta" -> [1,0,5]）。
func splitSemver(s string) []int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		nums[i], _ = strconv.Atoi(p)
	}
	return nums
}
