// Package app 装配核心模块并管理公共生命周期，平台入口只负责配置和退出信号。
package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extservice"
	extspec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	accountpkg "github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/admin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/config"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/event"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/janitor"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/module"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/task"
	"github.com/ShadowSmallBaby/ClawProxyHub/web"
	"gorm.io/gorm"
)

type App struct {
	deviceSetup                bool
	noWeb                      bool
	systemComponents           func() []extension.SystemComponent
	extensionTrust             extspec.TrustStore
	runtimeValidator           func(context.Context, string) error
	nativeRuntimeManagement    bool
	webHandler                 http.Handler
	admin                      *admin.Server
	externalScheduler          bool
	actions                    *action.Registry
	extensions                 *extension.Manager
	extensionServices          *extensionServices
	extensionConnector         func(context.Context, string, string, int) (*extservice.Connection, error)
	cfg                        config.Config
	registry                   *module.Registry
	routes                     routeHost
	db                         *gorm.DB
	settings                   *setting.Store
	plugins                    *plugin.Manager
	pluginRuntime              plugin.Runtime
	accounts                   *accountpkg.Service
	engine                     *task.Engine
	bus                        *event.Bus
	refreshDone, retentionDone <-chan struct{}
	server                     *http.Server
	listener                   net.Listener
	serveErrors                chan error
}

type Option func(*App)

func WithoutWeb() Option { return func(a *App) { a.noWeb = true } }

func WithDeviceSetup() Option { return func(a *App) { a.deviceSetup = true } }

func WithSystemComponents(query func() []extension.SystemComponent) Option {
	return func(a *App) { a.systemComponents = query }
}

func WithExtensionTrust(trust extspec.TrustStore) Option {
	return func(a *App) { a.extensionTrust = trust }
}

// WithExtensionConnector 由平台连接已验签的 Go 扩展入口，连接不接收任何业务凭据。
func WithExtensionConnector(connect func(context.Context, string, string, int) (*extservice.Connection, error)) Option {
	return func(a *App) { a.extensionConnector = connect }
}

// WithRuntimeValidator 由平台检查本机服务、SDK 与原生包签名约束。
func WithRuntimeValidator(check func(context.Context, string) error) Option {
	return func(a *App) { a.runtimeValidator = check }
}

func WithWeb(handler http.Handler) Option { return func(a *App) { a.webHandler = handler } }

// WithPluginRuntime 由平台宿主提供已验证的插件连接，业务模块无需感知启动方式。
func WithPluginRuntime(runtime plugin.Runtime) Option {
	return func(a *App) { a.pluginRuntime = runtime }
}

func WithExternalScheduler() Option { return func(a *App) { a.externalScheduler = true } }

func New(cfg *config.Config, options ...Option) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("missing application config")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &App{cfg: *cfg, routes: routeHost{mux: http.NewServeMux()}, serveErrors: make(chan error, 1)}
	for _, option := range options {
		option(a)
	}
	if a.cfg.Profile == "" {
		a.cfg.Profile = "full"
	}
	modules := a.modules()
	if a.noWeb {
		filtered := modules[:0]
		for _, m := range modules {
			if m.Descriptor().ID != "web" {
				filtered = append(filtered, m)
			}
		}
		modules = filtered
	}
	modules = append(modules, a.gatewayModule())
	var dependencies []string
	for _, impl := range modules {
		dependencies = append(dependencies, impl.Descriptor().ID)
	}
	modules = append(modules, a.httpModule(dependencies))
	registry, err := module.New(modules...)
	if err != nil {
		return nil, err
	}
	a.registry = registry
	return a, nil
}

func (a *App) Start(ctx context.Context) error    { return a.registry.Start(ctx, &a.routes) }
func (a *App) Shutdown(ctx context.Context) error { return a.registry.Stop(ctx) }
func (a *App) States() []module.State             { return a.registry.States() }
func (a *App) Address() string {
	if a.listener == nil {
		return ""
	}
	return a.listener.Addr().String()
}

func Run(ctx context.Context, cfg *config.Config, options ...Option) error {
	a, err := New(cfg, options...)
	if err != nil {
		return err
	}
	if err := a.Start(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
	case err = <-a.serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return errors.Join(err, a.Shutdown(stopCtx))
}

func (a *App) modules() []module.Module {
	return []module.Module{
		a.extensionModule(),
		&module.Func{Spec: module.Descriptor{ID: "storage", Capabilities: []string{"storage", "settings", "events"}},
			RegisterFunc: func(module.Host) error {
				if err := os.MkdirAll(a.cfg.DataDir, 0755); err != nil {
					return err
				}
				if err := os.MkdirAll(a.cfg.PluginDir, 0755); err != nil {
					return err
				}
				path := database.DSNToFilepath(a.cfg.DatabaseDSN)
				if err := database.ApplyPendingRestore(path, a.cfg.DataDir); err != nil {
					return err
				}
				var err error
				a.db, err = database.Open(context.Background(), a.cfg.DatabaseDSN)
				if err != nil {
					return err
				}
				if err := accountpkg.InitCrypto(a.cfg.DataDir); err != nil {
					return err
				}
				if err := accountpkg.EncryptProxyPasswords(a.db, a.cfg.DataDir); err != nil {
					return err
				}
				if key := os.Getenv("CPH_SEED_API_KEY"); key != "" {
					if err := seedAPIKey(a.db, key, a.cfg.DataDir); err != nil {
						return err
					}
				}
				a.settings, a.bus = setting.New(a.db), event.New()
				return nil
			}, StopFunc: func(context.Context) error {
				if a.db == nil {
					return nil
				}
				db, err := a.db.DB()
				if err != nil {
					return err
				}
				return db.Close()
			}},
		&module.Func{Spec: module.Descriptor{ID: "plugins", Requires: []string{"storage"}, Capabilities: []string{"plugins"}},
			RegisterFunc: func(module.Host) error {
				a.plugins = plugin.NewManager(a.cfg.PluginDir, a.db, plugin.WithRuntime(a.pluginRuntime))
				a.plugins.Configure(a.settings, func(blob []byte) ([]byte, error) { return accountpkg.DecryptCredential(a.cfg.DataDir, blob) })
				return nil
			}, StopFunc: func(context.Context) error {
				if a.plugins != nil {
					a.plugins.StopAll()
				}
				return nil
			}},
		&module.Func{Spec: module.Descriptor{ID: "plugin-startup", Requires: []string{"plugins", "extensions"}}, StartFunc: func(ctx context.Context) error {
			bins, err := a.plugins.AutoStarts()
			if err != nil {
				fmt.Printf("[plugin] list startup state failed: %v\n", err)
			}
			for _, bin := range bins {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if _, err := a.plugins.Start(ctx, bin); err != nil {
					fmt.Printf("[plugin] start failed: %v\n", err)
				}
			}
			a.plugins.RefreshCatalog(ctx)
			return ctx.Err()
		}, StopFunc: func(context.Context) error {
			if a.plugins != nil {
				a.plugins.StopAll()
			}
			return nil
		}},
		&module.Func{Spec: module.Descriptor{ID: "accounts", Requires: []string{"plugin-startup"}, Capabilities: []string{"accounts", "instances", "groups", "proxies", "oauth"}},
			RegisterFunc: func(module.Host) error {
				a.accounts = accountpkg.New(a.db, a.cfg.DataDir, a.plugins, a.settings)
				return nil
			},
			StartFunc: func(ctx context.Context) error { a.refreshDone = a.accounts.SubscribeRefresh(ctx, a.bus); return nil },
			StopFunc:  func(ctx context.Context) error { return wait(ctx, a.refreshDone) }},
		&module.Func{Spec: module.Descriptor{ID: "tasks", Requires: []string{"accounts"}, Capabilities: []string{"tasks"}},
			RegisterFunc: func(module.Host) error {
				a.engine = task.NewEngine(a.db, a.cfg.DataDir, task.NewPluginRunner(a.plugins), a.bus, a.settings)
				if a.externalScheduler {
					a.engine.UseExternalScheduler()
				}
				return nil
			},
			StartFunc: func(ctx context.Context) error { a.engine.Start(ctx); return nil },
			StopFunc: func(ctx context.Context) error {
				if a.engine == nil {
					return nil
				}
				return a.engine.Shutdown(ctx)
			}},
		&module.Func{Spec: module.Descriptor{ID: "maintenance", Requires: []string{"storage"}, Capabilities: []string{"retention"}},
			StartFunc: func(ctx context.Context) error {
				a.retentionDone = janitor.StartLogRetention(ctx, a.db, a.settings)
				return nil
			},
			StopFunc: func(ctx context.Context) error { return wait(ctx, a.retentionDone) }},
		&module.Func{Spec: module.Descriptor{ID: "admin", Requires: []string{"tasks", "extensions"}, Capabilities: []string{"admin", "auth", "logs"}},
			RegisterFunc: func(host module.Host) error {
				srv := admin.New(a.db, a.accounts, a.plugins, a.engine, a.settings, a.cfg.MarketplaceURL, a.cfg.DataDir, database.DSNToFilepath(a.cfg.DatabaseDSN), admin.WithModules(a.cfg.Profile, a.States), admin.WithExtensions(a.extensions, a.actions))
				admin.WithSystemComponents(a.systemComponents)(srv)
				if a.deviceSetup {
					admin.WithDeviceSetup()(srv)
				}
				a.admin = srv
				if err := host.Handle("GET /assets/plugins/{name}/icon", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path, ok := a.plugins.IconFile(r.PathValue("name"))
					if !ok {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Cache-Control", "public, max-age=3600")
					http.ServeFile(w, r, path)
				})); err != nil {
					return err
				}
				return host.Handle("/admin/", srv.Handler())
			}},
		&module.Func{Spec: module.Descriptor{ID: "web", Requires: []string{"admin"}, Capabilities: []string{"web"}},
			RegisterFunc: func(host module.Host) error {
				if a.webHandler != nil {
					return host.Handle("/", a.webHandler)
				}
				return host.Handle("/", web.Handler())
			}},
	}
}

func (a *App) httpModule(dependencies []string) module.Module {
	return &module.Func{Spec: module.Descriptor{ID: "http", Requires: dependencies, Capabilities: []string{"http"}},
		RegisterFunc: func(host module.Host) error {
			return host.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"status":"ok"}`))
			}))
		}, StartFunc: func(context.Context) error {
			listener, err := net.Listen("tcp", a.cfg.Addr)
			if err != nil {
				return err
			}
			a.listener = listener
			a.server = &http.Server{Handler: adminCORS(a.cfg.AdminOrigins, a.routes.mux), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
			go func() { a.serveErrors <- a.server.Serve(listener) }()
			fmt.Printf("listening on %s\n", listener.Addr())
			return nil
		}, StopFunc: func(ctx context.Context) error {
			if a.server == nil {
				return nil
			}
			err := a.server.Shutdown(ctx)
			if err != nil {
				err = errors.Join(err, a.server.Close())
			}
			if a.listener != nil {
				if closeErr := a.listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
					err = errors.Join(err, closeErr)
				}
			}
			return err
		}}
}

func wait(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type routeHost struct{ mux *http.ServeMux }

func (h *routeHost) Handle(pattern string, handler http.Handler) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("register route %q: %v", pattern, v)
		}
	}()
	h.mux.Handle(pattern, handler)
	return nil
}

// ExecuteDue 由系统调度或宿主前台工作调用，等待本次领取的任务结束。
func (a *App) ExecuteDue(ctx context.Context) error {
	if err := a.engine.TriggerDue(ctx); err != nil {
		return err
	}
	return a.engine.WaitIdle(ctx)
}

func (a *App) DeviceSession() (string, error) {
	return a.admin.DeviceSession()
}

// RefreshPlatformPlugin 重建一个平台插件会话，保留业务配置与历史。
func (a *App) RefreshPlatformPlugin(ctx context.Context, name string) error {
	a.plugins.Stop(name, false)
	return a.SyncPlatformPlugins(ctx)
}

// SyncPlatformPlugins 同步平台可用状态，不删除插件业务配置。
func (a *App) SyncPlatformPlugins(ctx context.Context) error {
	dirs, err := a.plugins.AutoStarts()
	if err != nil {
		return err
	}
	available := map[string]bool{}
	for _, dir := range dirs {
		available[filepath.Base(dir)] = true
		if _, err := a.plugins.Start(ctx, dir); err != nil {
			fmt.Printf("[platform] plugin unavailable: %v\n", err)
		}
	}
	for _, name := range a.plugins.Names() {
		if !available[name] {
			a.plugins.Stop(name, false)
		}
	}
	a.plugins.RefreshCatalog(ctx)
	return nil
}
