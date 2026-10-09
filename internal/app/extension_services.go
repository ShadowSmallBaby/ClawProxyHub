package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	ext "github.com/ShadowSmallBaby/ClawProxyHub/internal/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extservice"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extstore"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type serviceBinding struct {
	hash    string
	process *extservice.Process
	storage *extstore.Session
	ready   *atomic.Bool
}
type extensionServices struct {
	mu       sync.Mutex
	store    *extstore.Store
	bindings map[string]serviceBinding
}

func (a *App) configureExtensionServices() error {
	store, err := extstore.Open(filepath.Join(a.cfg.DataDir, "cph.ext.db"))
	if err != nil {
		return err
	}
	a.extensionServices = &extensionServices{store: store, bindings: map[string]serviceBinding{}}
	a.extensions.SetDataHooks(ext.DataHooks{
		Clear: store.Clear, Obsolete: store.Obsolete, Snapshot: store.Snapshot,
		Prepare: func(ctx context.Context, state ext.State) error {
			if state.Manifest.Storage == nil {
				return nil
			}
			session, err := store.Bind(ctx, state.Manifest.ID, state.Signer, state.Hash, *state.Manifest.Storage)
			if err == nil {
				session.Close()
			}
			return err
		},
		Committed: func(states []ext.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			installed := make([]extstore.Installed, 0, len(states))
			for _, state := range states {
				installed = append(installed, extstore.Installed{ID: state.Manifest.ID, Hash: state.Hash, Storage: state.Manifest.Storage})
			}
			if err := store.SyncInstalled(ctx, installed); err != nil {
				return err
			}
			a.extensionServices.mu.Lock()
			defer a.extensionServices.mu.Unlock()
			for _, state := range states {
				binding := a.extensionServices.bindings[state.Manifest.ID]
				if binding.ready != nil && binding.hash == state.Hash && state.Enabled && state.Available {
					binding.ready.Store(true)
					if binding.process != nil {
						binding.process.Commit()
					}
				}
			}
			return nil
		},
	})
	return nil
}

func (a *App) activateExtension(ctx context.Context, s ext.State) (err error) {
	if s.Manifest.Kind == "runtime" {
		return a.plugins.SetLuaRuntime(func() (string, error) { return a.extensions.Executable(s) })
	}
	binding := serviceBinding{hash: s.Hash, ready: &atomic.Bool{}}
	defer func() {
		if err != nil {
			if binding.process != nil {
				_ = binding.process.Close(context.Background())
			}
			if binding.storage != nil {
				binding.storage.Close()
			}
			a.extensionServices.mu.Lock()
			delete(a.extensionServices.bindings, s.Manifest.ID)
			a.extensionServices.mu.Unlock()
		}
	}()
	if s.Manifest.Storage != nil {
		binding.storage, err = a.extensionServices.store.Bind(ctx, s.Manifest.ID, s.Signer, s.Hash, *s.Manifest.Storage)
		if err != nil {
			return err
		}
	}
	if s.Manifest.Backend != nil {
		path, err := a.extensions.Executable(s)
		if err != nil {
			return err
		}
		values := map[string]any{}
		raw, found, err := a.settings.Lookup(setting.ExtensionConfigKey(s.Manifest.ID))
		if err != nil {
			return err
		}
		if found {
			if err = json.Unmarshal([]byte(raw), &values); err != nil {
				return err
			}
		}
		settings := map[string]any{}
		for _, field := range s.Manifest.Settings {
			value, ok := values[field.ID]
			if field.Readonly || !ok || field.ValidateValue(value) != nil {
				value = field.DefaultValue()
			}
			settings[field.ID] = value
		}
		onExit := func(exited *extservice.Process, cause error) {
			recovery, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := a.extensions.BackendExited(recovery, s.Manifest.ID, s.Hash, cause, func() bool {
				a.extensionServices.mu.Lock()
				defer a.extensionServices.mu.Unlock()
				return a.extensionServices.bindings[s.Manifest.ID].process == exited
			}); err != nil {
				log.Printf("[extension] %s exit cleanup: %v", s.Manifest.ID, err)
			}
		}
		init := spec.Initialize{Protocol: spec.BackendProtocol, ID: s.Manifest.ID, Version: s.Manifest.Version, PackageHash: s.Hash, Actions: s.Manifest.Actions, Settings: settings}
		if runtime.GOOS == "android" {
			if a.extensionConnector == nil || s.Manifest.Backend.Android == nil {
				return fmt.Errorf("Android extension executor is unavailable")
			}
			entry := s.Manifest.Backend.Entries[runtime.GOOS+"/"+runtime.GOARCH]
			connection, connectErr := a.extensionConnector(ctx, path, s.Manifest.Files[entry], s.Manifest.Backend.Android.MinSDK)
			if connectErr != nil {
				return connectErr
			}
			binding.process, err = extservice.StartConnection(ctx, connection, init, s.Manifest.Backend.BackgroundPermissions, binding.storage, onExit)
		} else {
			binding.process, err = extservice.Start(ctx, path, init, s.Manifest.Backend.BackgroundPermissions, binding.storage, onExit)
		}
		if err != nil {
			return err
		}
	}
	descriptors := []action.Descriptor{}
	handlers := []action.Handler{}
	for _, declaration := range s.Manifest.Actions {
		var descriptor action.Descriptor
		var handler action.Handler
		if declaration.Target == "" {
			if binding.process == nil {
				return fmt.Errorf("backend action has no process")
			}
			descriptor = action.Descriptor{Version: 1, Permission: declaration.Permission, Effect: declaration.Effect, TimeoutMS: declaration.TimeoutMS, Input: *declaration.Input, Output: *declaration.Output}
			handler = func(ctx context.Context, p action.Principal, input json.RawMessage) (any, error) {
				grants := extensionGrants(p, s.Manifest.Permissions, declaration.Effect == "read")
				return binding.process.Invoke(ctx, declaration.ID, input, grants)
			}
		} else if strings.HasPrefix(declaration.Target, "host.storage.") {
			if binding.storage == nil {
				return fmt.Errorf("storage action requires declared tables")
			}
			op := strings.TrimPrefix(declaration.Target, "host.storage.")
			if !spec.HasPermission([]string{"query", "insert", "update", "delete", "batch"}, op) {
				return fmt.Errorf("unsupported storage action")
			}
			permission, effect := "storage.write", "write"
			if op == "query" {
				permission, effect = "storage.read", "read"
			}
			if !spec.HasPermission(s.Manifest.Permissions, permission) {
				return fmt.Errorf("missing storage action permission")
			}
			descriptor = action.Descriptor{Version: 1, Permission: permission, Effect: effect, TimeoutMS: 5000, Input: spec.Schema{Type: "object", AdditionalProperties: true}, Output: spec.Schema{Type: "object", AdditionalProperties: true}}
			handler = func(ctx context.Context, p action.Principal, input json.RawMessage) (any, error) {
				var requests []spec.StorageRequest
				if op == "batch" {
					var in struct {
						Requests []spec.StorageRequest `json:"requests"`
					}
					if err := spec.DecodeStrict(input, &in); err != nil {
						return nil, err
					}
					requests = in.Requests
				} else {
					var request spec.StorageRequest
					if err := spec.DecodeStrict(input, &request); err != nil {
						return nil, err
					}
					if request.Op != "" && request.Op != op {
						return nil, fmt.Errorf("storage operation mismatch")
					}
					request.Op = op
					requests = []spec.StorageRequest{request}
				}
				results, err := binding.storage.Execute(ctx, requests, extensionGrants(p, s.Manifest.Permissions, effect == "read"))
				return map[string]any{"results": results}, err
			}
		} else {
			var target *action.Descriptor
			for _, d := range a.actions.List(action.Principal{Role: "admin"}) {
				if d.Owner == "core" && d.ID == declaration.Target {
					copy := d
					target = &copy
					break
				}
			}
			if target == nil {
				return fmt.Errorf("unknown core action %s", declaration.Target)
			}
			if target.Permission == "extensions.manage" {
				return fmt.Errorf("extension lifecycle actions cannot be aliased by an extension")
			}
			if !spec.HasPermission(s.Manifest.Permissions, target.Permission) {
				return fmt.Errorf("missing extension grant for %s", target.Permission)
			}
			descriptor = *target
			targetID := target.ID
			handler = func(ctx context.Context, p action.Principal, input json.RawMessage) (any, error) {
				p.Scopes = extensionGrants(p, s.Manifest.Permissions, false)
				return a.actions.Invoke(ctx, p, targetID, input)
			}
		}
		descriptor.ID = s.Manifest.ID + "." + declaration.ID
		descriptor.Owner = s.Manifest.ID
		descriptor.Title = declaration.Title
		bound := handler
		handler = func(ctx context.Context, p action.Principal, input json.RawMessage) (any, error) {
			state, exists := a.extensions.State(s.Manifest.ID)
			if !exists || !state.Available || state.Hash != s.Hash || !binding.ready.Load() {
				return nil, action.ErrUnavailable
			}
			return bound(ctx, p, input)
		}
		descriptors = append(descriptors, descriptor)
		handlers = append(handlers, handler)
	}
	if err = a.actions.RegisterBatch(descriptors, handlers); err != nil {
		return err
	}
	if binding.process != nil && !binding.process.Alive() {
		_ = a.actions.RemoveOwner(ctx, s.Manifest.ID)
		return fmt.Errorf("extension exited before activation")
	}
	a.extensionServices.mu.Lock()
	a.extensionServices.bindings[s.Manifest.ID] = binding
	a.extensionServices.mu.Unlock()
	return nil
}

func extensionGrants(p action.Principal, declared []string, readOnly bool) []string {
	grants := []string{}
	for _, permission := range declared {
		if p.Allows(permission) && (!readOnly || permission != "storage.write") {
			grants = append(grants, permission)
		}
	}
	return grants
}
func (a *App) deactivateExtension(ctx context.Context, id string) error {
	if id == "lua-runtime" {
		return a.plugins.SetLuaRuntime(nil)
	}
	err := a.actions.RemoveOwner(ctx, id)
	a.extensionServices.mu.Lock()
	binding := a.extensionServices.bindings[id]
	delete(a.extensionServices.bindings, id)
	a.extensionServices.mu.Unlock()
	if binding.process != nil {
		err = errors.Join(err, binding.process.Close(ctx))
	}
	if binding.storage != nil {
		binding.storage.Close()
	}
	return err
}
