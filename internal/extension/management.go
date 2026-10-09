package extension

import (
	"context"
	"path/filepath"
	"strings"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

// SetManagementPolicy 在公共安装状态上划分平台管理入口，所有变更共用此检查。
func (m *Manager) SetManagementPolicy(check func(context.Context, spec.Manifest) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.management = check
}

func (m *Manager) CheckManagement(ctx context.Context, manifest spec.Manifest) error {
	m.mu.RLock()
	check := m.management
	m.mu.RUnlock()
	if check != nil {
		return check(ctx, manifest)
	}
	return nil
}

func (m *Manager) ListFor(ctx context.Context) []State {
	result := []State{}
	for _, state := range m.List() {
		if m.CheckManagement(ctx, state.Manifest) == nil {
			result = append(result, state)
		}
	}
	return result
}

func (m *Manager) RuntimeManagement(ctx context.Context) string {
	if m.CheckManagement(ctx, spec.Manifest{Kind: "runtime"}) != nil {
		return "native"
	}
	return "core"
}

func (m *Manager) CatalogFor(ctx context.Context) []CatalogEntry {
	result := []CatalogEntry{}
	for _, item := range m.Catalog() {
		manifest := spec.Manifest{}
		if item.Manifest != nil {
			manifest = *item.Manifest
		} else if strings.EqualFold(filepath.Ext(item.Path), ".cphhost") {
			manifest.Kind = "runtime"
		}
		if m.CheckManagement(ctx, manifest) == nil {
			result = append(result, item)
		}
	}
	return result
}
