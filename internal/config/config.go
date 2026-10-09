// Package config — 核心运行配置：环境变量 > 默认值。
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
)

// Config 是核心进程的运行配置。
type Config struct {
	// 发行标识；仅支持 full，空则使用默认值。
	Profile string
	// HTTP 监听地址
	Addr string
	// 独立客户端访问管理 API 的来源白名单（精确 origin）。
	AdminOrigins []string
	// 数据目录（SQLite / 插件目录都在其下）
	DataDir string
	// 数据库 DSN；空则用 <DataDir>/cph.db
	DatabaseDSN string
	// 插件安装目录；空则用 <DataDir>/plugins
	PluginDir string
	// 挂载的 .cphext/.cphhost 包目录；空则使用 <DataDir>/packages。
	PackageDirs []string
	// 仅关闭挂载包的首次自动安装，已有安装和手动安装仍可用。
	DisablePackageAutoInstall bool
	// 插件市场索引 URL（仪表盘「设置」里的自建地址优先于此项）
	MarketplaceURL string
}

// Load 从环境变量读取配置，未设置项用默认值填充。
func Load() (*Config, error) {
	cfg := &Config{
		Profile:        os.Getenv("CPH_PROFILE"),
		Addr:           envStr("CPH_ADDR", ":8080"),
		DataDir:        envStr("CPH_DATA_DIR", "./data"),
		MarketplaceURL: envStr("CPH_MARKETPLACE_URL", setting.DefaultMarketplaceURL),
	}
	install, err := strconv.ParseBool(envStr("CPH_INSTALL_PACKAGES", "true"))
	if err != nil {
		return nil, fmt.Errorf("CPH_INSTALL_PACKAGES must be true or false: %w", err)
	}
	cfg.DisablePackageAutoInstall = !install
	for _, origin := range strings.Split(os.Getenv("CPH_ADMIN_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.AdminOrigins = append(cfg.AdminOrigins, origin)
		}
	}
	cfg.DatabaseDSN = envStr("CPH_DATABASE_DSN", cfg.DataDir+"/cph.db")
	cfg.PluginDir = envStr("CPH_PLUGIN_DIR", cfg.DataDir+"/plugins")
	for _, dir := range filepath.SplitList(os.Getenv("CPH_PACKAGE_DIRS")) {
		if dir != "" {
			cfg.PackageDirs = append(cfg.PackageDirs, dir)
		}
	}
	return cfg, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Validate 检查配置合法性。
func (c *Config) Validate() error {
	for _, origin := range c.AdminOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.Contains(origin, "#") {
			return fmt.Errorf("invalid admin origin %q (expected scheme://host[:port])", origin)
		}
		loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
		if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
			return fmt.Errorf("admin origin %q requires HTTPS unless loopback", origin)
		}
	}
	if c.Profile != "" && c.Profile != "full" {
		return fmt.Errorf("unknown profile %q (expected full)", c.Profile)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data dir must not be empty")
	}
	return nil
}
