// cph — 平台入口只处理配置、信号与退出码。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/app"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/config"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/distribution"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dir := flag.String("distribution", "", "verified distribution directory (default beside executable)")
	install := flag.Bool("install-packages", !cfg.DisablePackageAutoInstall, "automatically install new mounted packages (CPH_INSTALL_PACKAGES)")
	flag.Parse()
	cfg.DisablePackageAutoInstall = !*install
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if *dir == "" {
		candidate := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(candidate, "distribution.lock.json")); err == nil {
			*dir = candidate
		}
	}
	var options []app.Option
	if *dir != "" {
		*dir, err = filepath.Abs(*dir)
		if err != nil {
			return err
		}
		b, err := distribution.Open(*dir, exe, version.Core)
		if err != nil {
			return err
		}
		if cfg.Profile != "" && cfg.Profile != b.Profile() {
			return fmt.Errorf("CPH_PROFILE conflicts with distribution lock")
		}
		cfg.Profile = b.Profile()
		if os.Getenv("CPH_DATA_DIR") == "" {
			cfg.DataDir = filepath.Join(*dir, "data")
			if os.Getenv("CPH_DATABASE_DSN") == "" {
				cfg.DatabaseDSN = filepath.Join(cfg.DataDir, "cph.db")
			}
			if os.Getenv("CPH_PLUGIN_DIR") == "" {
				cfg.PluginDir = filepath.Join(cfg.DataDir, "plugins")
			}
		}
		if len(cfg.PackageDirs) == 0 {
			cfg.PackageDirs = []string{filepath.Join(cfg.DataDir, "packages")}
		}
		mounted := false
		for _, candidate := range cfg.PackageDirs {
			path, _ := filepath.Abs(candidate)
			mounted = mounted || path == b.PackageDir()
		}
		if !mounted {
			cfg.PackageDirs = append(cfg.PackageDirs, b.PackageDir())
		}
		options = append(options, app.WithExtensionTrust(b.Lock.Trust))
	}
	return app.Run(ctx, cfg, options...)
}
