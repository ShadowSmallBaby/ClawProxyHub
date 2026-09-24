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
			"first_token_timeout":      int(s.settings.FirstTokenTimeout().Seconds()),
			"first_event_timeout":      int(s.settings.FirstEventTimeout().Seconds()),
			"max_retries":              s.settings.MaxRetries(),
			"user_agent":               s.settings.GatewayUserAgent(),
			"browser_user_agent":       s.settings.BrowserUserAgent(),
			"github_proxy":             s.settings.GitHubProxy(),
			"log_retention_days":       s.settings.LogRetentionDays(),
			"run_level":                s.settings.RunLevel(),
			"task_daily_jitter":        int(s.settings.DailyJitter().Minutes()),
			"context_truncate_enabled": s.settings.ContextTruncateEnabled(),
			"context_truncate_ratio":   s.settings.ContextTruncateRatio(),
			"context_bytes_per_token":  s.settings.ContextBytesPerToken(),
			"plugin_lua_enabled":       s.settings.LuaEnabled(),
			"plugin_lua_isolation":     s.settings.LuaIsolation(),
			"plugin_lua_update_mode":   s.settings.LuaUpdateMode(),
			"site_name":                s.settings.Get(setting.KeySiteName, ""), // 原值：空 = 默认，前端用 placeholder 提示
			"site_abbr":                s.settings.Get(setting.KeySiteAbbr, ""),
			"site_logo":                s.settings.SiteLogo(),
		},
	})
}

// putSettings PUT /admin/settings — body: {first_token_timeout, user_agent, browser_user_agent, github_proxy, log_retention_days, site_name, site_abbr, site_logo}
// （插件源见 /admin/plugin-sources）。前端按 tab 分块保存，缺省字段保持原值。
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FirstTokenTimeout      *int     `json:"first_token_timeout"`
		FirstEventTimeout      *int     `json:"first_event_timeout"`
		MaxRetries             *int     `json:"max_retries"`
		UserAgent              *string  `json:"user_agent"`
		BrowserUserAgent       *string  `json:"browser_user_agent"`
		GitHubProxy            *string  `json:"github_proxy"`
		LogRetentionDays       *int     `json:"log_retention_days"`
		RunLevel               *string  `json:"run_level"`
		TaskDailyJitter        *int     `json:"task_daily_jitter"`
		ContextTruncateEnabled *bool    `json:"context_truncate_enabled"`
		ContextTruncateRatio   *float64 `json:"context_truncate_ratio"`
		ContextBytesPerToken   *float64 `json:"context_bytes_per_token"`
		LuaEnabled             *bool    `json:"plugin_lua_enabled"`
		LuaIsolation           *bool    `json:"plugin_lua_isolation"`
		LuaUpdateMode          *string  `json:"plugin_lua_update_mode"`
		SiteName               *string  `json:"site_name"`
		SiteAbbr               *string  `json:"site_abbr"`
		SiteLogo               *string  `json:"site_logo"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if body.FirstTokenTimeout != nil {
		if *body.FirstTokenTimeout < 5 || *body.FirstTokenTimeout > 3600 {
			http.Error(w, `{"error":"首字超时需在 5–3600 秒之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyFirstTokenTimeout, strconv.Itoa(*body.FirstTokenTimeout))
	}
	if body.FirstEventTimeout != nil {
		if *body.FirstEventTimeout < 5 || *body.FirstEventTimeout > 3600 {
			http.Error(w, `{"error":"首帧超时需在 5–3600 秒之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyFirstEventTimeout, strconv.Itoa(*body.FirstEventTimeout))
	}
	if body.MaxRetries != nil {
		if *body.MaxRetries < 1 || *body.MaxRetries > 10 {
			http.Error(w, `{"error":"重试次数需在 1–10 之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyMaxRetries, strconv.Itoa(*body.MaxRetries))
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
	if body.RunLevel != nil {
		switch *body.RunLevel {
		case "error", "warn", "debug", "info":
			s.settings.Set(setting.KeyRunLevel, *body.RunLevel)
		default:
			http.Error(w, `{"error":"运行日志级别需为 error/warn/debug/info"}`, http.StatusBadRequest)
			return
		}
	}
	// 任务偏移：daily 触发的最大随机抖动分钟数，0 = 关闭偏移
	if body.TaskDailyJitter != nil {
		if *body.TaskDailyJitter < 0 || *body.TaskDailyJitter > 45 {
			http.Error(w, `{"error":"任务偏移分钟数需在 0（关闭）–45 之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyTaskDailyJitter, strconv.Itoa(*body.TaskDailyJitter))
	}
	// 输入超窗自动截断：开关 + 触发比例(0,1] + token 估算系数
	if body.ContextTruncateEnabled != nil {
		s.settings.Set(setting.KeyContextTruncateEnabled, strconv.FormatBool(*body.ContextTruncateEnabled))
	}
	if body.ContextTruncateRatio != nil {
		if *body.ContextTruncateRatio <= 0 || *body.ContextTruncateRatio > 1 {
			http.Error(w, `{"error":"上下文截断阈值需在 (0,1] 之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyContextTruncateRatio, strconv.FormatFloat(*body.ContextTruncateRatio, 'g', -1, 64))
	}
	if body.ContextBytesPerToken != nil {
		if *body.ContextBytesPerToken < 1 || *body.ContextBytesPerToken > 100 {
			http.Error(w, `{"error":"token 估算系数需在 1–100 之间"}`, http.StatusBadRequest)
			return
		}
		s.settings.Set(setting.KeyContextBytesPerToken, strconv.FormatFloat(*body.ContextBytesPerToken, 'g', -1, 64))
	}
	// 插件（lua 运行时）：启用总开关 / 隔离（本版锁定为开）/ 更新方式
	if body.LuaEnabled != nil {
		s.settings.Set(setting.KeyLuaEnabled, strconv.FormatBool(*body.LuaEnabled))
	}
	if body.LuaIsolation != nil { // 本版锁定为开：无论传入何值都存 true（前端 disabled，此为服务端兜底）
		s.settings.Set(setting.KeyLuaIsolation, strconv.FormatBool(true))
	}
	if body.LuaUpdateMode != nil {
		switch *body.LuaUpdateMode {
		case "manual", "online":
			s.settings.Set(setting.KeyLuaUpdateMode, *body.LuaUpdateMode)
		default:
			http.Error(w, `{"error":"lua 更新方式需为 manual/online"}`, http.StatusBadRequest)
			return
		}
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
