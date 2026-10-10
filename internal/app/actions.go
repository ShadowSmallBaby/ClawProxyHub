package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// registerCoreActions 为所有客户端共享业务规则和审计，处理器不依赖 HTTP。
func (a *App) registerCoreActions() error {
	object := action.Schema{Type: "object", AdditionalProperties: true}
	empty := action.Schema{Type: "object"}
	idInput := action.Schema{Type: "object", Properties: map[string]action.Schema{"id": {Type: "integer"}}, Required: []string{"id"}}
	extensionID := action.Schema{Type: "object", Properties: map[string]action.Schema{"id": {Type: "string", MaxLength: 96}}, Required: []string{"id"}}
	entries := []struct {
		id, title, permission, effect string
		input                         action.Schema
		handler                       action.Handler
	}{
		{"status", "Core status", "status.read", "read", empty, func(context.Context, action.Principal, json.RawMessage) (any, error) {
			return map[string]any{"profile": a.cfg.Profile, "modules": a.States()}, nil
		}},
		{"extensions.impact", "Extension removal impact", "extensions.read", "read", extensionID, func(_ context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID string }
			json.Unmarshal(raw, &in)
			return a.extensionImpact(in.ID)
		}},
		{"tasks.list", "List task rules", "tasks.read", "read", empty, func(ctx context.Context, _ action.Principal, _ json.RawMessage) (any, error) {
			var rules []struct {
				ID           int64  `json:"id"`
				PluginID     int64  `json:"plugin_id"`
				CapabilityID string `json:"capability_id"`
				Enabled      bool   `json:"enabled"`
			}
			err := a.db.WithContext(ctx).Model(&model.TaskRule{}).Select("id,plugin_id,capability_id,enabled").Order("id").Limit(1000).Find(&rules).Error
			return map[string]any{"rules": rules}, err
		}},
		{"tasks.run", "Queue task rule", "tasks.execute", "execute", idInput, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID int64 }
			json.Unmarshal(raw, &in)
			if in.ID < 1 {
				return nil, fmt.Errorf("invalid rule ID")
			}
			var rule model.TaskRule
			if err := a.db.WithContext(ctx).First(&rule, in.ID).Error; err != nil {
				return nil, err
			}
			id, err := a.engine.RunNow(ctx, &rule)
			return map[string]any{"run_id": id}, err
		}},
		{"tasks.cancel", "Cancel task rule", "tasks.execute", "execute", idInput, func(_ context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID int64 }
			json.Unmarshal(raw, &in)
			if in.ID < 1 {
				return nil, fmt.Errorf("invalid rule ID")
			}
			err := a.engine.CancelRule(in.ID)
			return map[string]any{"cancelled": err == nil}, err
		}},
		{"extensions.list", "List extensions", "extensions.read", "read", empty, func(ctx context.Context, _ action.Principal, _ json.RawMessage) (any, error) {
			result := map[string]any{"extensions": a.extensions.ListFor(ctx), "system_components": []any{}, "runtime_management": a.extensions.RuntimeManagement(ctx)}
			if a.systemComponents != nil {
				result["system_components"] = a.systemComponents()
			}
			return result, nil
		}},
		{"extensions.install", "Install verified upload", "extensions.manage", "write", action.Schema{Type: "object", Properties: map[string]action.Schema{"ticket": {Type: "string", MaxLength: 64}, "grants": {Type: "array", MaxItems: 64, Items: &action.Schema{Type: "string", MaxLength: 96}}}, Required: []string{"ticket", "grants"}}, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct {
				Ticket string
				Grants []string
			}
			json.Unmarshal(raw, &in)
			return a.extensions.InstallUpload(ctx, in.Ticket, in.Grants)
		}},
		{"extensions.settings", "Read extension settings", "extensions.manage", "read", extensionID, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID string }
			json.Unmarshal(raw, &in)
			return a.extensions.Settings(ctx, in.ID)
		}},
		{"extensions.obsolete", "Review obsolete extension tables", "extensions.read", "read", extensionID, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID string }
			_ = json.Unmarshal(raw, &in)
			return a.extensions.ObsoleteTables(ctx, in.ID, false, "")
		}},
		{"extensions.clean-obsolete", "Clean obsolete extension tables", "extensions.manage", "write", action.Schema{Type: "object", Properties: map[string]action.Schema{"id": {Type: "string", MaxLength: 96}, "hash": {Type: "string", MaxLength: 64}}, Required: []string{"id", "hash"}}, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID, Hash string }
			_ = json.Unmarshal(raw, &in)
			return a.extensions.ObsoleteTables(ctx, in.ID, true, in.Hash)
		}},
		{"extensions.configure", "Save extension settings", "extensions.manage", "write", action.Schema{Type: "object", Properties: map[string]action.Schema{"id": {Type: "string", MaxLength: 96}, "hash": {Type: "string", MaxLength: 64}, "values": object}, Required: []string{"id", "hash", "values"}}, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct {
				ID, Hash string
				Values   map[string]any
			}
			json.Unmarshal(raw, &in)
			return a.extensions.UpdateSettings(ctx, in.ID, in.Hash, in.Values)
		}},
	}
	for _, op := range []string{"enable", "disable", "uninstall", "cache", "data"} {
		entries = append(entries, struct {
			id, title, permission, effect string
			input                         action.Schema
			handler                       action.Handler
		}{"extensions." + op, "Extension " + op, "extensions.manage", "write", extensionID, func(ctx context.Context, _ action.Principal, raw json.RawMessage) (any, error) {
			var in struct{ ID string }
			json.Unmarshal(raw, &in)
			var err error
			switch op {
			case "enable":
				err = a.extensions.SetEnabled(ctx, in.ID, true)
			case "disable":
				err = a.extensions.SetEnabled(ctx, in.ID, false)
			case "uninstall":
				err = a.extensions.Uninstall(ctx, in.ID)
			case "cache":
				err = a.extensions.CleanCache(ctx, in.ID)
			case "data":
				err = a.extensions.CleanData(ctx, in.ID)
			}
			return map[string]any{"ok": err == nil}, err
		}})
	}
	for _, e := range entries {
		if err := a.actions.Register(action.Descriptor{ID: "core." + e.id, Owner: "core", Version: 1, Title: e.title, Permission: e.permission, Effect: e.effect, TimeoutMS: 30000, Input: e.input, Output: object}, e.handler); err != nil {
			return err
		}
	}
	return nil
}
