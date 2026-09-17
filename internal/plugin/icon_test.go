package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIconFile(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, nil)

	// 插件目录：manifest 声明 icon 且文件存在
	demo := filepath.Join(dir, "demo")
	os.MkdirAll(demo, 0o755)
	os.WriteFile(filepath.Join(demo, "manifest.json"), []byte(`{"name":"demo","icon":"icon.png"}`), 0o644)
	os.WriteFile(filepath.Join(demo, "icon.png"), []byte("png"), 0o644)
	if p, ok := m.IconFile("demo"); !ok || filepath.Base(p) != "icon.png" {
		t.Fatalf("icon not resolved: %v %v", p, ok)
	}

	// 路径穿越 / 不存在的插件 / 未声明 icon / 文件缺失
	if _, ok := m.IconFile("../etc"); ok {
		t.Fatal("path traversal allowed")
	}
	if _, ok := m.IconFile("missing"); ok {
		t.Fatal("missing plugin resolved")
	}
	noIcon := filepath.Join(dir, "noicon")
	os.MkdirAll(noIcon, 0o755)
	os.WriteFile(filepath.Join(noIcon, "manifest.json"), []byte(`{"name":"noicon"}`), 0o644)
	if _, ok := m.IconFile("noicon"); ok {
		t.Fatal("icon resolved without declaration")
	}
	noFile := filepath.Join(dir, "nofile")
	os.MkdirAll(noFile, 0o755)
	os.WriteFile(filepath.Join(noFile, "manifest.json"), []byte(`{"name":"nofile","icon":"icon.png"}`), 0o644)
	if _, ok := m.IconFile("nofile"); ok {
		t.Fatal("icon resolved without file")
	}
}
