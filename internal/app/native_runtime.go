package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type nativeRuntimeContext struct{}

// WithNativeRuntimeManagement 将运行时管理交给平台私有入口，HTTP 与动作接口仅管理功能扩展。
func WithNativeRuntimeManagement() Option {
	return func(a *App) { a.nativeRuntimeManagement = true }
}

func nativeRuntimePolicy(ctx context.Context, manifest spec.Manifest) error {
	if (manifest.Kind == "runtime" || manifest.ID == "lua-runtime") && ctx.Value(nativeRuntimeContext{}) != true {
		return fmt.Errorf("manage runtimes through the native application page")
	}
	return nil
}

func (a *App) runtimeContext(ctx context.Context) context.Context {
	if a.nativeRuntimeManagement {
		return context.WithValue(ctx, nativeRuntimeContext{}, true)
	}
	return ctx
}

type NativeRuntimeRequest struct {
	Operation string         `json:"operation"`
	ID        string         `json:"id"`
	File      string         `json:"file"`
	SHA256    string         `json:"sha256"`
	Grants    []string       `json:"grants"`
	Values    map[string]any `json:"values"`
}

// NativeRuntime 仅由平台进程内调用，不注册为 HTTP 路由或公共动作。
func (a *App) NativeRuntime(ctx context.Context, request NativeRuntimeRequest) (any, error) {
	if !a.nativeRuntimeManagement || a.extensions == nil {
		return nil, fmt.Errorf("native runtime management is unavailable")
	}
	ctx = a.runtimeContext(ctx)
	if request.Operation == "list" {
		installed := []extension.State{}
		for _, state := range a.extensions.List() {
			if state.Manifest.Kind == "runtime" {
				installed = append(installed, state)
			}
		}
		mounted := []extension.CatalogEntry{}
		for _, item := range a.extensions.Catalog() {
			if item.Manifest != nil && item.Manifest.Kind == "runtime" || item.Manifest == nil && strings.EqualFold(filepath.Ext(item.Path), ".cphhost") {
				mounted = append(mounted, item)
			}
		}
		return map[string]any{"installed": installed, "packages": mounted}, nil
	}
	if request.Operation == "inspect" || request.Operation == "install" {
		verified, err := a.extensions.Inspect(request.File)
		if err != nil {
			return nil, err
		}
		if verified.Manifest.ID != "lua-runtime" || verified.Manifest.Kind != "runtime" {
			return nil, fmt.Errorf("select a Lua Host runtime package")
		}
		if request.Operation == "inspect" {
			return map[string]any{"manifest": verified.Manifest, "sha256": verified.SHA256, "publisher": verified.Publisher}, nil
		}
		if request.SHA256 == "" || request.SHA256 != verified.SHA256 {
			return nil, fmt.Errorf("runtime package differs from the inspected content")
		}
		return a.extensions.InstallExpected(ctx, request.File, request.Grants, request.SHA256)
	}
	if request.ID != "lua-runtime" {
		return nil, fmt.Errorf("unsupported runtime identity")
	}
	if request.Operation == "mounted" {
		for _, item := range a.extensions.Catalog() {
			if item.SHA256 == request.SHA256 && item.Manifest != nil && item.Manifest.ID == request.ID && item.Manifest.Kind == "runtime" {
				return a.extensions.InstallMounted(ctx, request.SHA256, request.Grants)
			}
		}
		return nil, fmt.Errorf("runtime package is no longer mounted")
	}
	var err error
	switch request.Operation {
	case "settings":
		return a.extensions.Settings(ctx, request.ID)
	case "configure":
		return a.extensions.UpdateSettings(ctx, request.ID, request.SHA256, request.Values)
	case "enable", "disable":
		err = a.extensions.SetEnabled(ctx, request.ID, request.Operation == "enable")
	case "uninstall":
		err = a.extensions.Uninstall(ctx, request.ID)
	case "impact":
		return a.extensionImpact(request.ID)
	default:
		err = fmt.Errorf("unsupported native runtime operation")
	}
	return map[string]bool{"ok": err == nil}, err
}
