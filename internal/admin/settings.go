// settings.go — 系统设置 API（网关全局参数）。
package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
)

// getSettings GET /admin/settings
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"settings": map[string]interface{}{
			"first_event_timeout": int(s.settings.FirstEventTimeout().Seconds()),
			"github_proxy":        s.settings.GitHubProxy(),
			"marketplace_url":     s.marketURL(),
		},
	})
}

// putSettings PUT /admin/settings — body: {first_event_timeout, github_proxy, marketplace_url}。
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FirstEventTimeout int    `json:"first_event_timeout"`
		GitHubProxy       string `json:"github_proxy"`
		MarketplaceURL    string `json:"marketplace_url"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if body.FirstEventTimeout < 5 || body.FirstEventTimeout > 3600 {
		http.Error(w, `{"error":"首事件超时需在 5–3600 秒之间"}`, http.StatusBadRequest)
		return
	}
	proxy := strings.TrimSuffix(strings.TrimSpace(body.GitHubProxy), "/")
	if proxy != "" && !strings.HasPrefix(proxy, "http://") && !strings.HasPrefix(proxy, "https://") {
		http.Error(w, `{"error":"GitHub 代理需以 http:// 或 https:// 开头（如 https://ghproxy.com），留空则直连"}`, http.StatusBadRequest)
		return
	}
	marketURL := strings.TrimSpace(body.MarketplaceURL)
	if marketURL != "" && !strings.HasPrefix(marketURL, "http://") && !strings.HasPrefix(marketURL, "https://") {
		http.Error(w, `{"error":"插件市场地址需以 http:// 或 https:// 开头（指向 index.json），留空则用官方地址"}`, http.StatusBadRequest)
		return
	}
	s.settings.Set(setting.KeyFirstEventTimeout, strconv.Itoa(body.FirstEventTimeout))
	s.settings.Set(setting.KeyGitHubProxy, proxy)
	s.settings.Set(setting.KeyMarketplaceURL, marketURL)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
