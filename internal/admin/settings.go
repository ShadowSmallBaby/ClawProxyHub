// settings.go — 系统设置 API（网关 / 网络 / 日志保留 / 站点品牌）。
package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
)

// 站点品牌字段上限：logo 为 data URL（base64 后约 ×1.37），限 256KB。
const (
	maxSiteNameLen = 32
	maxSiteAbbrLen = 8
	maxSiteLogoLen = 256 << 10
)

// branding 站点品牌视图（登录页 / 侧栏用，免鉴权）。
func (s *Server) branding() map[string]string {
	return map[string]string{
		"name": s.settings.SiteName(),
		"abbr": s.settings.SiteAbbr(),
		"logo": s.settings.SiteLogo(),
	}
}

// getBranding GET /admin/branding — 站点品牌（免鉴权，登录页也要展示）。
func (s *Server) getBranding(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.branding())
}

// getSettings GET /admin/settings
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"settings": map[string]interface{}{
			"first_event_timeout": int(s.settings.FirstEventTimeout().Seconds()),
			"user_agent":          s.settings.GatewayUserAgent(),
			"browser_user_agent":  s.settings.BrowserUserAgent(),
			"github_proxy":        s.settings.GitHubProxy(),
			"log_retention_days":  s.settings.LogRetentionDays(),
			"site_name":           s.settings.Get(setting.KeySiteName, ""), // 原值：空 = 默认，前端用 placeholder 提示
			"site_abbr":           s.settings.Get(setting.KeySiteAbbr, ""),
			"site_logo":           s.settings.SiteLogo(),
		},
	})
}

// putSettings PUT /admin/settings — body: {first_event_timeout, user_agent, browser_user_agent, github_proxy, log_retention_days, site_name, site_abbr, site_logo}
// （插件源见 /admin/plugin-sources）。前端按 tab 分块保存，缺省字段保持原值。
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FirstEventTimeout *int    `json:"first_event_timeout"`
		UserAgent         *string `json:"user_agent"`
		BrowserUserAgent  *string `json:"browser_user_agent"`
		GitHubProxy       *string `json:"github_proxy"`
		LogRetentionDays  *int    `json:"log_retention_days"`
		SiteName          *string `json:"site_name"`
		SiteAbbr          *string `json:"site_abbr"`
		SiteLogo          *string `json:"site_logo"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if body.FirstEventTimeout != nil {
		if *body.FirstEventTimeout < 5 || *body.FirstEventTimeout > 3600 {
			http.Error(w, `{"error":"首事件超时需在 5–3600 秒之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyFirstEventTimeout, strconv.Itoa(*body.FirstEventTimeout))
	}
	// UA：空 = 不覆盖（网关 UA 透传客户端；浏览器 UA 由插件用内置值）
	for key, v := range map[string]*string{setting.KeyGatewayUserAgent: body.UserAgent, setting.KeyBrowserUserAgent: body.BrowserUserAgent} {
		if v == nil {
			continue
		}
		ua := strings.TrimSpace(*v)
		if len(ua) > 512 {
			http.Error(w, `{"error":"User-Agent 不超过 512 字符"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(key, ua)
	}
	if body.GitHubProxy != nil {
		proxy := strings.TrimSuffix(strings.TrimSpace(*body.GitHubProxy), "/")
		if proxy != "" && !strings.HasPrefix(proxy, "http://") && !strings.HasPrefix(proxy, "https://") {
			http.Error(w, `{"error":"GitHub 代理需以 http:// 或 https:// 开头（如 https://ghproxy.com），留空则直连"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyGitHubProxy, proxy)
	}
	if body.LogRetentionDays != nil {
		if *body.LogRetentionDays < 0 || *body.LogRetentionDays > 3650 {
			http.Error(w, `{"error":"日志保留天数需在 0（永久）–3650 之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyLogRetentionDays, strconv.Itoa(*body.LogRetentionDays))
	}
	// 站点品牌：空串 = 恢复默认（存空，读取时回退）
	if body.SiteName != nil {
		name := strings.TrimSpace(*body.SiteName)
		if len([]rune(name)) > maxSiteNameLen {
			http.Error(w, `{"error":"站点品牌名不超过 32 字符"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeySiteName, name)
	}
	if body.SiteAbbr != nil {
		abbr := strings.TrimSpace(*body.SiteAbbr)
		if len([]rune(abbr)) > maxSiteAbbrLen {
			http.Error(w, `{"error":"站点缩写不超过 8 字符"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeySiteAbbr, abbr)
	}
	if body.SiteLogo != nil {
		logo := strings.TrimSpace(*body.SiteLogo)
		if logo != "" && !strings.HasPrefix(logo, "data:image/") {
			http.Error(w, `{"error":"logo 需为图片 data URL"}`, http.StatusBadRequest)
			return
		}
		if len(logo) > maxSiteLogoLen {
			http.Error(w, `{"error":"logo 不超过 256KB"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeySiteLogo, logo)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
