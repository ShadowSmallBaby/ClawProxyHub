// Package setting — 系统设置 KV（settings 表）读写，带进程内缓存。
package setting

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// KeyFirstTokenTimeout 网关首字超时（秒）：首事件 → 首个内容 token 的上限（容纳推理思考）。
const KeyFirstTokenTimeout = "gateway.first_token_timeout"

// KeyFirstEventTimeout 网关首帧超时（秒）：等待上游第一个事件的上限（探测上游/代理挂死）。
const KeyFirstEventTimeout = "gateway.first_event_timeout"

// KeyMaxRetries 单次请求的总上游尝试次数上限（含首发；恢复/降级共用此硬上限）。
const KeyMaxRetries = "gateway.max_retries"

// KeyGatewayUserAgent 网关全局 UA（对话请求；空 = 透传客户端 UA；路由级可覆盖）。
const KeyGatewayUserAgent = "gateway.user_agent"

// KeyBrowserUserAgent 全局浏览器 UA（供插件登录 / 管理面等浏览器形态请求；空 = 插件用内置值）。
const KeyBrowserUserAgent = "gateway.browser_user_agent"

// KeyGitHubProxy GitHub 代理前缀（ghproxy 风格，加速插件市场访问；空 = 直连）。
const KeyGitHubProxy = "network.github_proxy"

// KeyLogRetentionDays 调用日志保留天数（0 = 永久，不清理）。
const KeyLogRetentionDays = "logs.retention_days"

// KeyRunLevel 运行日志记录级别（error/warn/debug/info，递增包含；默认 error）。
const KeyRunLevel = "logs.run_level"

// KeyTaskDailyJitter daily 任务触发的最大随机抖动分钟数（错开多账号同刻打上游；0 = 关闭偏移）。
const KeyTaskDailyJitter = "task.daily_jitter_minutes"

// DefaultTaskDailyJitter daily 抖动默认窗口（分钟）。
const DefaultTaskDailyJitter = 30

// 站点品牌（登录页 / 侧栏展示；空 = 内置默认）。
const (
	KeySiteName = "site.name" // 站点品牌名
	KeySiteAbbr = "site.abbr" // 站点缩写（未命名 API Key 的默认名等）
	KeySiteLogo = "site.logo" // 站点 logo，data URL；空 = 内置 /logo.png

	DefaultSiteName = "ClawProxyHub"
	DefaultSiteAbbr = "CPH"
)

// KeyMarketplaceURL 旧版单一市场地址（v1.1 起改为 KeyPluginSources 源列表；仅用于首次迁移导入）。
const KeyMarketplaceURL = "network.marketplace_url"

// KeyPluginSources 插件源列表（JSON 数组，见 PluginSource）。
const KeyPluginSources = "network.plugin_sources"

// OfficialSourceName 官方源名：该源的插件安装在插件根目录，其他源按源名建命名空间目录。
const OfficialSourceName = "official"

// DefaultMarketplaceURL 官方插件市场索引地址（初始化时写入设置）。
const DefaultMarketplaceURL = "https://raw.githubusercontent.com/ShadowSmallBaby/ClawProxyHubPlugins/main/index.json"

// PluginSource 一个插件市场源（index.json 地址）。
type PluginSource struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

const defaultFirstTokenTimeout = 120
const defaultFirstEventTimeout = 60
const defaultMaxRetries = 3

// KeyContextTruncateEnabled 输入超窗自动截断总开关（默认开）。
const KeyContextTruncateEnabled = "gateway.context_truncate_enabled"

// KeyContextTruncateRatio 触发截断的窗口占用阈值比例（估算 token / context_window 超过即截）。
const KeyContextTruncateRatio = "gateway.context_truncate_ratio"

// KeyContextBytesPerToken token 粗估系数：字节数 / 该值 ≈ token 数。
const KeyContextBytesPerToken = "gateway.context_bytes_per_token"

const (
	defaultContextTruncateEnabled = true
	defaultContextTruncateRatio   = 0.9
	defaultContextBytesPerToken   = 3.5
)

// Store 设置存储。
type Store struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]string
}

func New(db *gorm.DB) *Store {
	return &Store{db: db, cache: map[string]string{}}
}

// Get 读设置，缺省返回 def。
func (s *Store) Get(key, def string) string {
	s.mu.RLock()
	v, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		return v
	}
	var rec model.Setting
	if err := s.db.Where("key = ?", key).First(&rec).Error; err != nil {
		return def
	}
	s.mu.Lock()
	s.cache[key] = rec.Value
	s.mu.Unlock()
	return rec.Value
}

// Set 写设置（upsert + 刷新缓存）。
func (s *Store) Set(key, value string) {
	s.db.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, value)
	s.mu.Lock()
	s.cache[key] = value
	s.mu.Unlock()
}

// FirstTokenTimeout 网关首字超时；非法值回退默认。
func (s *Store) FirstTokenTimeout() time.Duration {
	n, err := strconv.Atoi(s.Get(KeyFirstTokenTimeout, strconv.Itoa(defaultFirstTokenTimeout)))
	if err != nil || n <= 0 {
		n = defaultFirstTokenTimeout
	}
	return time.Duration(n) * time.Second
}

// FirstEventTimeout 网关首帧超时；非法值回退默认。
func (s *Store) FirstEventTimeout() time.Duration {
	n, err := strconv.Atoi(s.Get(KeyFirstEventTimeout, strconv.Itoa(defaultFirstEventTimeout)))
	if err != nil || n <= 0 {
		n = defaultFirstEventTimeout
	}
	return time.Duration(n) * time.Second
}

// MaxRetries 单次请求总上游尝试次数上限；非法值回退默认，下限 1。
func (s *Store) MaxRetries() int {
	n, err := strconv.Atoi(s.Get(KeyMaxRetries, strconv.Itoa(defaultMaxRetries)))
	if err != nil || n < 1 {
		n = defaultMaxRetries
	}
	return n
}

// ContextTruncateEnabled 输入超窗自动截断开关；缺省开。
func (s *Store) ContextTruncateEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(s.Get(KeyContextTruncateEnabled, ""))) {
	case "false", "0", "off", "no":
		return false
	}
	return defaultContextTruncateEnabled
}

// ContextTruncateRatio 截断触发比例；非法或越界回退默认，限定 (0,1]。
func (s *Store) ContextTruncateRatio() float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s.Get(KeyContextTruncateRatio, "")), 64)
	if err != nil || f <= 0 || f > 1 {
		return defaultContextTruncateRatio
	}
	return f
}

// ContextBytesPerToken token 粗估系数；非法或过小回退默认。
func (s *Store) ContextBytesPerToken() float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s.Get(KeyContextBytesPerToken, "")), 64)
	if err != nil || f < 1 {
		return defaultContextBytesPerToken
	}
	return f
}

// GitHubProxy GitHub 代理前缀（以 / 结尾与否均可；空 = 直连）。
func (s *Store) GitHubProxy() string {
	return s.Get(KeyGitHubProxy, "")
}

// GatewayUserAgent 网关全局 UA（空 = 透传客户端）。
func (s *Store) GatewayUserAgent() string { return strings.TrimSpace(s.Get(KeyGatewayUserAgent, "")) }

// BrowserUserAgent 全局浏览器 UA（空 = 插件用内置值）。
func (s *Store) BrowserUserAgent() string { return strings.TrimSpace(s.Get(KeyBrowserUserAgent, "")) }

// LogRetentionDays 日志保留天数；非法值按永久（0）。
func (s *Store) LogRetentionDays() int {
	n, err := strconv.Atoi(s.Get(KeyLogRetentionDays, "0"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// RunLevel 运行日志记录级别；非法值回退 error。
func (s *Store) RunLevel() string {
	switch v := s.Get(KeyRunLevel, "error"); v {
	case "warn", "debug", "info":
		return v
	}
	return "error"
}

// DailyJitter daily 任务触发的最大随机抖动窗口；空/非法回退默认，0 = 关闭偏移。
func (s *Store) DailyJitter() time.Duration {
	n, err := strconv.Atoi(s.Get(KeyTaskDailyJitter, strconv.Itoa(DefaultTaskDailyJitter)))
	if err != nil || n < 0 {
		n = DefaultTaskDailyJitter
	}
	return time.Duration(n) * time.Minute
}

// SiteName 站点品牌名（空回退默认）。
func (s *Store) SiteName() string {
	if v := strings.TrimSpace(s.Get(KeySiteName, "")); v != "" {
		return v
	}
	return DefaultSiteName
}

// SiteAbbr 站点缩写（空回退默认）。
func (s *Store) SiteAbbr() string {
	if v := strings.TrimSpace(s.Get(KeySiteAbbr, "")); v != "" {
		return v
	}
	return DefaultSiteAbbr
}

// SiteLogo 自定义 logo data URL（空 = 用内置）。
func (s *Store) SiteLogo() string { return s.Get(KeySiteLogo, "") }

// PluginSources 插件源列表（解析失败返回空）。
func (s *Store) PluginSources() []PluginSource {
	var out []PluginSource
	_ = json.Unmarshal([]byte(s.Get(KeyPluginSources, "[]")), &out)
	return out
}

// SetPluginSources 写插件源列表。
func (s *Store) SetPluginSources(list []PluginSource) {
	b, _ := json.Marshal(list)
	s.Set(KeyPluginSources, string(b))
}

// EnsurePluginSources 首次迁移：源列表缺失时用官方地址（config 默认）建 official 源；
// 旧版 marketplace_url 若为用户自建地址，一并导入为 custom 源。
func (s *Store) EnsurePluginSources(officialURL string) {
	var rec model.Setting
	if err := s.db.Where("key = ?", KeyPluginSources).First(&rec).Error; err == nil {
		return
	}
	list := []PluginSource{{Name: OfficialSourceName, URL: officialURL, Enabled: true}}
	if legacy := strings.TrimSpace(s.Get(KeyMarketplaceURL, "")); legacy != "" && legacy != officialURL {
		list = append(list, PluginSource{Name: "custom", URL: legacy, Enabled: true})
	}
	s.SetPluginSources(list)
}

// EnsureDefault key 尚未写入时落默认值（仅初始化场景使用，不覆盖已有配置）。
func (s *Store) EnsureDefault(key, def string) {
	var rec model.Setting
	if err := s.db.Where("key = ?", key).First(&rec).Error; err != nil {
		s.Set(key, def)
	}
}
