package extension

import (
	"context"
	"encoding/json"
	"fmt"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

const MaxSettingsBytes = 64 << 10

type Settings struct {
	Hash   string         `json:"hash"`
	Fields []spec.Setting `json:"fields"`
	Values map[string]any `json:"values"`
}

// SetSettingsStore 将用户配置交给宿主持久化；nil 配置表示显式清理数据。
func (m *Manager) SetSettingsStore(load func(string) (map[string]any, error), save func(string, map[string]any) error) {
	m.op.Lock()
	defer m.op.Unlock()
	m.loadSettings, m.saveSettings = load, save
}

func (m *Manager) storedSettings(id string) (map[string]any, error) {
	if m.loadSettings == nil {
		return map[string]any{}, nil
	}
	values, err := m.loadSettings(id)
	if values == nil && err == nil {
		values = map[string]any{}
	}
	return values, err
}

func effectiveSettings(state State, stored map[string]any) Settings {
	result := Settings{Hash: state.Hash, Fields: append([]spec.Setting{}, state.Manifest.Settings...), Values: map[string]any{}}
	for _, field := range state.Manifest.Settings {
		value, exists := stored[field.ID]
		if field.Readonly || !exists || field.ValidateValue(value) != nil {
			value = field.DefaultValue()
		}
		result.Values[field.ID] = value
	}
	return result
}

func (m *Manager) Settings(ctx context.Context, id string) (Settings, error) {
	m.op.Lock()
	defer m.op.Unlock()
	state, exists := m.State(id)
	if !exists {
		return Settings{}, fmt.Errorf("extension not installed")
	}
	if err := m.CheckManagement(ctx, state.Manifest); err != nil {
		return Settings{}, err
	}
	stored, err := m.storedSettings(id)
	if err != nil {
		return Settings{}, err
	}
	return effectiveSettings(state, stored), nil
}

// UpdateSettings 绑定已查看的包摘要，拒绝未知字段和只读项修改；升级保留仍适用的配置。
func (m *Manager) UpdateSettings(ctx context.Context, id, hash string, values map[string]any) (Settings, error) {
	m.op.Lock()
	defer m.op.Unlock()
	state, exists := m.State(id)
	if !exists {
		return Settings{}, fmt.Errorf("extension not installed")
	}
	if err := m.CheckManagement(ctx, state.Manifest); err != nil {
		return Settings{}, err
	}
	if hash != state.Hash {
		return Settings{}, fmt.Errorf("extension package changed; reopen settings")
	}
	data, err := json.Marshal(values)
	if err != nil || values == nil || len(data) > MaxSettingsBytes {
		return Settings{}, fmt.Errorf("invalid extension settings")
	}
	stored, err := m.storedSettings(id)
	if err != nil {
		return Settings{}, err
	}
	fields := map[string]spec.Setting{}
	for _, field := range state.Manifest.Settings {
		fields[field.ID] = field
	}
	for id, value := range values {
		field, exists := fields[id]
		if !exists {
			return Settings{}, fmt.Errorf("unknown extension setting %q", id)
		}
		if err := field.ValidateValue(value); err != nil {
			return Settings{}, err
		}
		if field.Readonly && value != field.DefaultValue() {
			return Settings{}, fmt.Errorf("setting %q is readonly", id)
		}
	}
	// 复制后再写入，校验或保存失败不改变当前配置。
	next := make(map[string]any, len(stored)+len(values))
	for id, value := range stored {
		next[id] = value
	}
	for id, value := range values {
		if !fields[id].Readonly {
			next[id] = value
		}
	}
	result := effectiveSettings(state, next)
	for _, field := range state.Manifest.Settings {
		if err := field.ValidateValue(result.Values[field.ID]); err != nil {
			return Settings{}, err
		}
	}
	data, err = json.Marshal(next)
	if err != nil || len(data) > MaxSettingsBytes {
		return Settings{}, fmt.Errorf("extension settings exceed storage limit")
	}
	if m.saveSettings == nil {
		return Settings{}, fmt.Errorf("extension settings storage unavailable")
	}
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	if err := m.saveSettings(id, next); err != nil {
		return Settings{}, err
	}
	return result, nil
}
