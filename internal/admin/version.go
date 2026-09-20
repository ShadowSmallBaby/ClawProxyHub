// version.go — 核心版本回显与检查更新。
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
)

// remoteVersionURL 远端版本清单（仓库根 version.json，经 GitHubProxy 加速）。
const remoteVersionURL = "https://raw.githubusercontent.com/ShadowSmallBaby/ClawProxyHub/main/version.json"

// releaseURL 版本发布页（前端「有更新」跳转）。
const releaseURL = "https://github.com/ShadowSmallBaby/ClawProxyHub/releases"

// coreVersion GET /admin/version — 本机版本 + 远端最新版对比（远端不可达时静默降级，只回本机版本）。
func (s *Server) coreVersion(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{"version": version.Core}
	if latest := s.fetchLatestVersion(); latest != "" {
		out["latest"] = latest
		out["update_available"] = latest != version.Core
		out["release_url"] = releaseURL
	}
	writeJSON(w, http.StatusOK, out)
}

// fetchLatestVersion 拉远端 version.json 的 version 字段；任何失败返回空（不阻塞前端）。
func (s *Server) fetchLatestVersion() string {
	req, err := http.NewRequest(http.MethodGet, s.withGitHubProxy(remoteVersionURL), nil)
	if err != nil {
		return ""
	}
	resp, err := marketHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return ""
	}
	return body.Version
}
