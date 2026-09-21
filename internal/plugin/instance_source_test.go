package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 设置合并：实例覆盖插件，base_url 最后写入。
func TestMergeJSON(t *testing.T) {
	dst := map[string]json.RawMessage{}
	mergeJSON(dst, `{"a":"plugin","b":"plugin"}`)
	mergeJSON(dst, `{"b":"instance","c":1}`)
	mergeJSON(dst, "not json") // 忽略
	mergeJSON(dst, "")
	out, _ := json.Marshal(dst)
	var got map[string]interface{}
	json.Unmarshal(out, &got)
	if got["a"] != "plugin" || got["b"] != "instance" || got["c"] != float64(1) {
		t.Fatalf("merge wrong: %v", got)
	}
}

// 目录定位：根目录优先，其次任意命名空间子目录；仅 manifest.json 存在才算插件目录。
func TestPluginDirAndScanNamespaces(t *testing.T) {
	root := t.TempDir()
	mk := func(parts ...string) {
		dir := filepath.Join(append([]string{root}, parts...)...)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"x"}`), 0o644)
		bin := filepath.Join(dir, "plugin-"+runtimeOS()+"-"+runtimeArch())
		if runtimeOS() == "windows" {
			bin += ".exe"
		}
		os.WriteFile(bin, []byte("bin"), 0o755)
	}
	mk("alpha")                                         // 官方源：根目录
	mk("mysrc", "beta")                                 // 自定义源：命名空间
	os.MkdirAll(filepath.Join(root, "empty-ns"), 0o755) // 空命名空间目录

	m := &Manager{dir: root}
	if d, ok := m.pluginDir("alpha"); !ok || d != filepath.Join(root, "alpha") {
		t.Fatalf("alpha: %q %v", d, ok)
	}
	if d, ok := m.pluginDir("beta"); !ok || d != filepath.Join(root, "mysrc", "beta") {
		t.Fatalf("beta: %q %v", d, ok)
	}
	if _, ok := m.pluginDir("mysrc"); ok {
		t.Fatal("namespace dir must not resolve as a plugin")
	}
	if _, ok := m.pluginDir("../alpha"); ok {
		t.Fatal("path traversal must be rejected")
	}
	bins, err := m.Scan()
	if err != nil || len(bins) != 2 {
		t.Fatalf("scan: %v %v", err, bins)
	}
}
