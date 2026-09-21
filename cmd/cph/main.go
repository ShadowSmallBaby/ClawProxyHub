// cph — ClawProxyHub 核心进程入口。
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/protobuf/encoding/protojson"

	accountpkg "github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/admin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/config"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/event"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/gateway"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/janitor"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/router"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/task"
	"github.com/ShadowSmallBaby/ClawProxyHub/web"
	"gorm.io/gorm"
)

// seedAPIKey 首次部署引导：环境变量指定 key，不存在则入库（加密存储）。
func seedAPIKey(db *gorm.DB, raw string, dataDir string) error {
	var count int64
	db.Model(&model.Key{}).Where("key_cipher = ?", string(accountpkg.EncryptCredential(dataDir, []byte(raw)))).Count(&count)
	if count > 0 {
		return nil
	}
	return db.Create(&model.Key{KeyCipher: string(accountpkg.EncryptCredential(dataDir, []byte(raw))), Name: "seed"}).Error
}

// syncPluginRecords 启动插件后同步 plugins 表（安装流程落地前的兜底）。
func syncPluginRecords(db *gorm.DB, plugins *plugin.Manager) {
	for _, name := range plugins.Names() {
		inst, ok := plugins.Get(name)
		if !ok {
			continue
		}
		m := inst.Manifest
		manifestJSON, _ := protojson.Marshal(m)
		var rec model.Plugin
		err := db.Where("name = ?", m.Name).First(&rec).Error
		if err != nil {
			db.Create(&model.Plugin{
				Name: m.Name, Version: m.Version, Author: m.Author,
				ProtocolVersion: m.ProtocolVersion, ManifestJSON: string(manifestJSON),
				Enabled: true,
			})
		} else {
			db.Model(&rec).Updates(map[string]interface{}{
				"version": m.Version, "author": m.Author,
				"protocol_version": m.ProtocolVersion, "manifest_json": string(manifestJSON),
			})
		}
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := os.MkdirAll(cfg.PluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	// 管理界面上传的备份在此换入（打开库之前）
	dbPath := database.DSNToFilepath(cfg.DatabaseDSN)
	if err := database.ApplyPendingRestore(dbPath, cfg.DataDir); err != nil {
		return err
	}
	db, err := database.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return err
	}

	if key := os.Getenv("CPH_SEED_API_KEY"); key != "" {
		if err := seedAPIKey(db, key, cfg.DataDir); err != nil {
			return err
		}
	}

	plugins := plugin.NewManager(cfg.PluginDir, db)
	if bins, err := plugins.Scan(); err == nil {
		for _, bin := range bins {
			if _, err := plugins.Start(ctx, bin); err != nil {
				fmt.Printf("[plugin] start failed: %v\n", err)
			}
		}
	}
	plugins.RefreshCatalog(ctx)
	syncPluginRecords(db, plugins)
	defer plugins.StopAll()

	bus := event.New()
	accounts := accountpkg.New(db, cfg.DataDir, plugins)
	accounts.SubscribeRefresh(ctx, bus)

	engine := task.NewEngine(db, cfg.DataDir, task.NewPluginRunner(plugins), bus)
	engine.Start(ctx)
	defer engine.Stop()
	settings := setting.New(db)
	janitor.StartLogRetention(ctx, db, settings)
	gw := gateway.New(db, cfg.DataDir, plugins, router.New(db), accounts, settings)
	adminSrv := admin.New(db, accounts, plugins, engine, settings, cfg.MarketplaceURL, cfg.DataDir, dbPath)

	mux := http.NewServeMux()
	mux.Handle("/v1/", gw.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/admin/", adminSrv.Handler())
	// 插件图标（img 标签带不了 Authorization，走免鉴权只读静态服务）
	mux.HandleFunc("GET /assets/plugins/{name}/icon", func(w http.ResponseWriter, r *http.Request) {
		path, ok := plugins.IconFile(r.PathValue("name"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, path)
	})
	mux.Handle("/", web.Handler())

	fmt.Printf("listening on %s\n", cfg.Addr)
	httpSrv := &http.Server{Addr: cfg.Addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		return httpSrv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}
