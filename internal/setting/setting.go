// Package setting — 系统设置 KV（settings 表）读写，带进程内缓存。
package setting

import (
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// KeyFirstEventTimeout 网关首事件超时（秒）。
const KeyFirstEventTimeout = "gateway.first_event_timeout"

// KeyGitHubProxy GitHub 代理前缀（ghproxy 风格，加速插件市场访问；空 = 直连）。
const KeyGitHubProxy = "network.github_proxy"

// KeyMarketplaceURL 插件市场索引地址（用户自建；空 = 官方默认）。
const KeyMarketplaceURL = "network.marketplace_url"

// DefaultMarketplaceURL 官方插件市场索引地址（初始化时写入设置）。
const DefaultMarketplaceURL = "https://raw.githubusercontent.com/ShadowSmallBaby/ClawProxyHubPlugins/main/index.json"

const defaultFirstEventTimeout = 90

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

// FirstEventTimeout 网关首事件超时；非法值回退默认。
func (s *Store) FirstEventTimeout() time.Duration {
	n, err := strconv.Atoi(s.Get(KeyFirstEventTimeout, strconv.Itoa(defaultFirstEventTimeout)))
	if err != nil || n <= 0 {
		n = defaultFirstEventTimeout
	}
	return time.Duration(n) * time.Second
}

// GitHubProxy GitHub 代理前缀（以 / 结尾与否均可；空 = 直连）。
func (s *Store) GitHubProxy() string {
	return s.Get(KeyGitHubProxy, "")
}

// MarketplaceURL 用户自建市场地址；空 = 未配置（回退官方默认）。
func (s *Store) MarketplaceURL() string {
	return s.Get(KeyMarketplaceURL, "")
}

// EnsureDefault key 尚未写入时落默认值（仅初始化场景使用，不覆盖已有配置）。
func (s *Store) EnsureDefault(key, def string) {
	var rec model.Setting
	if err := s.db.Where("key = ?", key).First(&rec).Error; err != nil {
		s.Set(key, def)
	}
}
