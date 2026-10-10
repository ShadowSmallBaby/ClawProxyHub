package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	ext "github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/module"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func (a *App) extensionModule() module.Module {
	return &module.Func{Spec: module.Descriptor{ID: "extensions", Requires: []string{"plugins"}, Capabilities: []string{"extensions", "actions", "workspace"}}, RegisterFunc: func(host module.Host) error {
		a.actions = action.New(func(v action.Audit) error {
			return a.db.Exec(`INSERT INTO action_audits(action_id,owner,subject,effect,started_at,duration_ms,outcome) VALUES(?,?,?,?,?,?,?)`, v.ID, v.Owner, v.Subject, v.Effect, v.Started, v.DurationMS, v.Outcome).Error
		})
		if err := a.registerWorkspace(); err != nil {
			return err
		}
		if err := a.registerCoreActions(); err != nil {
			return err
		}
		trust := a.extensionTrust
		if trust == nil {
			trust = spec.TrustStore{}
		}
		path := os.Getenv("CPH_EXTENSION_TRUST")
		if path == "" {
			path = filepath.Join(a.cfg.DataDir, "extensions-trust.json")
		}
		data, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(data) > 0 {
			if err = json.Unmarshal(data, &trust); err != nil {
				return fmt.Errorf("extension trust: %w", err)
			}
		}
		a.extensions = ext.New(a.cfg.DataDir, version.Core, trust)
		if err := a.configureExtensionServices(); err != nil {
			return err
		}
		a.extensions.SetSettingsStore(func(id string) (map[string]any, error) {
			raw, found, err := a.settings.Lookup(setting.ExtensionConfigKey(id))
			if err != nil || !found {
				return nil, err
			}
			var values map[string]any
			err = json.Unmarshal([]byte(raw), &values)
			return values, err
		}, func(id string, values map[string]any) error {
			key := setting.ExtensionConfigKey(id)
			if values == nil {
				return a.settings.Delete(key)
			}
			raw, err := json.Marshal(values)
			if err != nil {
				return err
			}
			return a.settings.Set(key, string(raw))
		})
		if a.nativeRuntimeManagement {
			a.extensions.SetManagementPolicy(nativeRuntimePolicy)
		}
		var managed spec.TrustStore
		if err = json.Unmarshal([]byte(a.settings.Get(setting.KeyExtensionTrust, "{}")), &managed); err != nil {
			return fmt.Errorf("managed extension trust: %w", err)
		}
		if err = a.extensions.ConfigureManagedTrust(managed, func(value spec.TrustStore) error {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			return a.settings.Set(setting.KeyExtensionTrust, string(data))
		}); err != nil {
			return err
		}
		dirs := a.cfg.PackageDirs
		if len(dirs) == 0 {
			dirs = []string{filepath.Join(a.cfg.DataDir, "packages")}
		}
		a.extensions.SetPackageDirs(dirs)
		a.extensions.SetRemoveGuard(a.guardExtensionRemoval)
		a.extensions.SetUpdateDependents(func(ctx context.Context, old ext.State) (ext.UpdateDependents, error) {
			if old.Manifest.ID == "lua-runtime" {
				return a.plugins.BeginLuaRuntimeUpdate(ctx)
			}
			return nil, nil
		})
		a.extensions.SetValidator(func(ctx context.Context, s ext.State) error {
			if s.Manifest.Kind == "runtime" {
				if s.Manifest.ID != "lua-runtime" || (s.Manifest.Execution != "trusted-process" && s.Manifest.Execution != "android-service") {
					return fmt.Errorf("unsupported runtime executor")
				}
				executable, err := a.extensions.Executable(s)
				if err != nil {
					return err
				}
				if a.runtimeValidator != nil {
					return a.runtimeValidator(ctx, a.extensions.Archive(s))
				}
				return plugin.ProbeLuaExecutable(ctx, executable)
			}
			if s.Manifest.Backend != nil {
				if _, err := a.extensions.Executable(s); err != nil {
					return err
				}
			}
			return a.extensionServices.store.Check(ctx, s.Manifest.ID, s.Signer, s.Manifest.Storage)
		})
		a.extensions.SetLifecycle(a.activateExtension, a.deactivateExtension)
		return host.Handle("GET /extension-assets/{id}/{hash}/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			content, err := a.extensions.ReadAsset(r.PathValue("id"), r.PathValue("hash"), r.PathValue("path"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'none'; form-action 'none'; base-uri 'none'")
			http.ServeContent(w, r, r.PathValue("path"), time.Time{}, content)
		}))
	}, StartFunc: func(ctx context.Context) error {
		legacy, _, err := a.settings.Lookup(setting.KeyLuaEnabled)
		if err != nil {
			return err
		}
		var disabled []string
		switch strings.ToLower(strings.TrimSpace(legacy)) {
		case "false", "0", "off", "no":
			disabled = append(disabled, "lua-runtime")
		}
		if err := a.extensions.Load(ctx, disabled...); err != nil {
			return err
		}
		if err := a.settings.Delete(setting.KeyLuaEnabled, setting.KeyLuaIsolation, setting.KeyLuaUpdateMode); err != nil {
			return err
		}
		if a.cfg.DisablePackageAutoInstall {
			return nil
		}
		return a.extensions.ImportMounted(a.runtimeContext(ctx))
	}, StopFunc: func(ctx context.Context) error {
		if a.extensions == nil {
			return nil
		}
		err := a.extensions.Close(ctx)
		if a.extensionServices != nil {
			err = errors.Join(err, a.extensionServices.store.Close())
		}
		return err
	}}
}

func (a *App) registerWorkspace() error {
	str := func(n int) action.Schema { return action.Schema{Type: "string", MaxLength: n} }
	obj := func(props map[string]action.Schema, required ...string) action.Schema {
		return action.Schema{Type: "object", Properties: props, Required: required}
	}
	empty := obj(nil)
	name := str(32)
	file := action.Schema{Type: "string", Enum: []string{"main.lua"}}
	okResult := obj(map[string]action.Schema{"saved": {Type: "boolean"}, "running": {Type: "boolean"}, "name": name, "content": str(2 << 20), "lua": str(2 << 20)})
	entries := []struct {
		id, title, permission, effect string
		schema                        action.Schema
		handler                       action.Handler
	}{
		{"workspace.scaffold", "Lua template", "workspace.read", "read", empty, func(context.Context, action.Principal, json.RawMessage) (any, error) {
			return map[string]any{"lua": a.plugins.ScaffoldLua("")}, nil
		}},
		{"workspace.read", "Read local source", "workspace.read", "read", obj(map[string]action.Schema{"name": name, "file": file}, "name", "file"), func(ctx context.Context, p action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ Name, File string }
			_ = json.Unmarshal(raw, &in)
			source, err := a.plugins.ReadLocalSource(in.Name, in.File)
			return map[string]any{"content": source}, err
		}},
		{"workspace.save", "Save local source", "workspace.write", "write", obj(map[string]action.Schema{"name": name, "file": file, "content": str(2 << 20)}, "name", "file", "content"), func(ctx context.Context, p action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ Name, File, Content string }
			_ = json.Unmarshal(raw, &in)
			err := a.plugins.SaveLocalSource(in.Name, in.File, in.Content)
			return map[string]any{"saved": err == nil}, err
		}},
		{"workspace.create", "Create local workspace", "workspace.write", "write", obj(map[string]action.Schema{"name": name, "label": str(128), "content": str(2 << 20), "icon": str(1 << 20)}, "name", "content"), func(ctx context.Context, p action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ Name, Label, Content, Icon string }
			_ = json.Unmarshal(raw, &in)
			icon, err := base64.StdEncoding.DecodeString(in.Icon)
			if err != nil {
				return nil, err
			}
			created, err := a.plugins.CreateLocalWorkspace(in.Name, in.Label, in.Content, icon)
			if err != nil {
				return nil, err
			}
			a.plugins.RefreshCatalog(ctx)
			return map[string]any{"saved": true, "name": created}, nil
		}},
		{"workspace.reload", "Run saved Lua source", "workspace.execute", "execute", obj(map[string]action.Schema{"name": name}, "name"), func(ctx context.Context, p action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ Name string }
			_ = json.Unmarshal(raw, &in)
			err := a.plugins.ReloadLocalSource(ctx, in.Name)
			return map[string]any{"running": err == nil}, err
		}},
	}
	for _, e := range entries {
		if err := a.actions.Register(action.Descriptor{ID: "core." + e.id, Version: 1, Owner: "core", Title: e.title, Permission: e.permission, Effect: e.effect, TimeoutMS: 30000, Input: e.schema, Output: okResult}, e.handler); err != nil {
			return err
		}
	}
	return nil
}
