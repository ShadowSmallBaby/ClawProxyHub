package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveLaunchLuaShared 验证 P1：lua 插件（目录内无 per-plugin 二进制）解析为
// 共享 luahost + `--dir <插件目录>`，插件名取自插件目录。
func TestResolveLaunchLuaShared(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dir: filepath.Join(root, "plugins")}
	pdir := filepath.Join(m.dir, "foo")
	_ = os.MkdirAll(pdir, 0o755)
	_ = os.WriteFile(filepath.Join(pdir, "manifest.json"), []byte(`{"name":"foo","runtime":"lua"}`), 0o644)
	_ = os.WriteFile(filepath.Join(pdir, "main.lua"), []byte(`return {}`), 0o644)
	// 放一个假的共享 luahost（stub 构建下 luahostBin 为空，走"共享文件存在"分支）
	_ = os.MkdirAll(m.hostsDir(), 0o755)
	_ = os.WriteFile(m.luahostFile(), []byte("fake-luahost"), 0o755)

	name, cmd, err := m.resolveLaunch(pdir)
	if err != nil {
		t.Fatalf("resolveLaunch: %v", err)
	}
	if name != "foo" {
		t.Errorf("name = %q, want foo", name)
	}
	if cmd.Path != m.luahostFile() {
		t.Errorf("cmd.Path = %q, want shared luahost %q", cmd.Path, m.luahostFile())
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--dir") || !strings.Contains(joined, pdir) {
		t.Errorf("cmd.Args = %v, want --dir %s", cmd.Args, pdir)
	}
	if !m.launchable(pdir) {
		t.Error("launchable should be true for lua plugin with shared luahost")
	}
}
