package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Lua 插件只能使用已验证的运行时提供者，停用后不回退到旧二进制。
func TestResolveLaunchLuaRuntimeProvider(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dir: filepath.Join(root, "plugins")}
	pdir := filepath.Join(m.dir, "foo")
	_ = os.MkdirAll(pdir, 0o755)
	_ = os.WriteFile(filepath.Join(pdir, "manifest.json"), []byte(`{"name":"foo","runtime":"lua"}`), 0o644)
	_ = os.WriteFile(filepath.Join(pdir, "main.lua"), []byte(`return {}`), 0o644)
	host := filepath.Join(root, "luahost")
	_ = os.WriteFile(host, []byte("fake-luahost"), 0o755)
	if _, _, err := m.resolveLaunch(pdir); err == nil {
		t.Fatal("loaded Lua without a runtime provider")
	}
	if err := m.SetLuaRuntime(func() (string, error) { return host, nil }); err != nil {
		t.Fatal(err)
	}

	name, cmd, err := m.resolveLaunch(pdir)
	if err != nil {
		t.Fatalf("resolveLaunch: %v", err)
	}
	if name != "foo" {
		t.Errorf("name = %q, want foo", name)
	}
	if cmd.Path != host {
		t.Errorf("cmd.Path = %q, want runtime %q", cmd.Path, host)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--dir") || !strings.Contains(joined, pdir) {
		t.Errorf("cmd.Args = %v, want --dir %s", cmd.Args, pdir)
	}
	if !m.launchable(pdir) {
		t.Error("launchable should be true for lua plugin with shared luahost")
	}
	m.plugins = map[string]*Instance{"foo": {Name: "foo", runtime: "lua"}}
	if err := m.SetLuaRuntime(nil); err == nil {
		t.Fatal("disabled a runtime still in use")
	}
	if _, _, err := m.resolveLaunch(pdir); err != nil {
		t.Fatalf("failed disable removed the active provider: %v", err)
	}
	delete(m.plugins, "foo")
	if err := m.SetLuaRuntime(nil); err != nil {
		t.Fatal(err)
	}
	if m.launchable(pdir) {
		t.Fatal("disabled Lua runtime remained launchable")
	}
	if _, _, err := m.resolveLaunch(pdir); err == nil {
		t.Fatal("disabled runtime fell back to a raw binary")
	}
}
