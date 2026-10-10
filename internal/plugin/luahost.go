// luahost.go 将 Lua 插件连接到扩展管理器提供的已验证运行时。
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func manifestRuntimeAt(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Runtime string `json:"runtime"`
	}
	_ = json.Unmarshal(data, &manifest)
	return manifest.Runtime
}

func (m *Manager) launchable(dir string) bool {
	if manifestRuntimeAt(dir) == "lua" {
		return m.LuaAvailable()
	}
	_, err := pluginBinary(dir)
	return err == nil
}

// LuaAvailable 运行能力只取决于扩展管理器激活的运行时。
func (m *Manager) LuaAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.luaProvider != nil
}

func (m *Manager) resolveLaunch(dir string) (string, *exec.Cmd, error) {
	name := filepath.Base(dir)
	if manifestRuntimeAt(dir) == "lua" {
		m.mu.RLock()
		provider := m.luaProvider
		m.mu.RUnlock()
		if provider == nil {
			return "", nil, fmt.Errorf("Lua Host is not installed or enabled; install a trusted .cphhost package")
		}
		host, err := provider()
		if err != nil {
			return "", nil, err
		}
		return name, execCommand(host, "--dir", dir), nil
	}
	bin, err := pluginBinary(dir)
	if err != nil {
		return "", nil, err
	}
	return name, execCommand(bin), nil
}

// SetLuaRuntime 与启动串行，存在使用者时拒绝替换或停用。
func (m *Manager) SetLuaRuntime(provider func() (string, error)) error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, instance := range m.plugins {
		if instance.runtime == "lua" {
			return fmt.Errorf("Lua runtime is required by running plugin %s; stop it first", name)
		}
	}
	m.luaProvider = provider
	return nil
}
