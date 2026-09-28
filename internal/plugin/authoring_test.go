package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// mkLocal 在 <root>/local/<name>/ 落一个自建插件（manifest.json + main.lua）。
func mkLocal(t *testing.T, root, name, manifest, lua string) {
	t.Helper()
	dir := filepath.Join(root, LocalNamespace, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(dir, "main.lua"), []byte(lua), 0o644)
}

// TestParseLuaIdentity 提取 PLUGIN_NAME / PLUGIN_LABEL_ZH；只认 ZH，不回退取 EN。
func TestParseLuaIdentity(t *testing.T) {
	content := `-- 头注释
local PLUGIN_NAME     = "my-plugin"   -- 插件名
local PLUGIN_LABEL_ZH = "我的插件"    -- 中文名
local PLUGIN_LABEL_EN = "My Plugin"   -- 英文名
local plugin = {}`
	name, label := ParseLuaIdentity(content)
	if name != "my-plugin" || label != "我的插件" {
		t.Fatalf("identity = %q/%q, want my-plugin/我的插件", name, label)
	}

	// 只有 EN、无 ZH：label(zh) 应为空（ParseLuaIdentity 精确取 PLUGIN_LABEL_ZH，不回退 EN）
	onlyEn := `local PLUGIN_NAME = "x"
local PLUGIN_LABEL_EN = "OnlyEnglish"`
	if _, l := ParseLuaIdentity(onlyEn); l != "" {
		t.Fatalf("ZH 缺失时 label 应为空，got %q", l)
	}
}

// TestLuaStringLiteral 从 `= "value" -- 注释` 形态提取引号内串。
func TestLuaStringLiteral(t *testing.T) {
	cases := map[string]string{
		`= "hello"  -- c`: "hello",
		`   =   "a b"`:    "a b",
		`= ""`:            "",
		`no quotes`:       "",
	}
	for in, want := range cases {
		if got := luaStringLiteral(in); got != want {
			t.Errorf("luaStringLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCutVarRest 精确匹配 `local <var>`，区分 PLUGIN_LABEL_ZH / _EN。
func TestCutVarRest(t *testing.T) {
	content := `local PLUGIN_LABEL_ZH = "中"
local PLUGIN_LABEL_EN = "En"`
	if got := luaStringLiteral(cutVarRest(content, "PLUGIN_LABEL_ZH")); got != "中" {
		t.Errorf("ZH = %q, want 中", got)
	}
	if got := luaStringLiteral(cutVarRest(content, "PLUGIN_LABEL_EN")); got != "En" {
		t.Errorf("EN = %q, want En", got)
	}
	if cutVarRest(content, "PLUGIN_NAME") != "" {
		t.Error("不存在的变量应返回空")
	}
	if got := cutVarRest(`local PLUGIN_NAMES = "x"`, "PLUGIN_NAME"); got != "" {
		t.Errorf("PLUGIN_NAME 误命中 PLUGIN_NAMES: rest=%q", got)
	}
}

// TestIsLocalPlugin local 命名空间下才算自建；根目录/其他命名空间/穿越/缺失均否。
func TestIsLocalPlugin(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dir: root}
	mkLocal(t, root, "mine", `{"name":"mine","runtime":"lua"}`, "return {}")
	// 根目录插件（非 local）
	os.MkdirAll(filepath.Join(root, "market"), 0o755)
	os.WriteFile(filepath.Join(root, "market", "manifest.json"), []byte(`{"name":"market"}`), 0o644)

	if !m.IsLocalPlugin("mine") {
		t.Error("local/mine 应为自建")
	}
	for _, n := range []string{"market", "missing", "../etc", "mine/.."} {
		if m.IsLocalPlugin(n) {
			t.Errorf("%q 不应判为自建", n)
		}
	}
}

// TestScaffoldRendering 脚手架 manifest 是合法 JSON 且 name/label 正确；lua 可被 ParseLuaIdentity 回读。
func TestScaffoldRendering(t *testing.T) {
	m := &Manager{}
	var mf PackageManifest
	if err := json.Unmarshal([]byte(m.ScaffoldManifestFile("foo", "福")), &mf); err != nil {
		t.Fatalf("manifest 非法 JSON: %v", err)
	}
	if mf.Name != "foo" || mf.Runtime != "lua" || mf.Label["zh"] != "福" {
		t.Fatalf("manifest 字段错误: %+v", mf)
	}
	lua := m.ScaffoldLua("福娃")
	if name, label := ParseLuaIdentity(lua); name == "" || label != "福娃" {
		t.Fatalf("脚手架 lua 身份回读错误: name=%q label=%q", name, label)
	}
}

// TestReadLocalSource 仅 main.lua 可读；非 main.lua / 非自建插件拒绝。
func TestReadLocalSource(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dir: root}
	mkLocal(t, root, "mine", `{"name":"mine","runtime":"lua"}`, "-- hi\nreturn {}")

	got, err := m.ReadLocalSource("mine", "main.lua")
	if err != nil || got != "-- hi\nreturn {}" {
		t.Fatalf("read main.lua: %q %v", got, err)
	}
	if _, err := m.ReadLocalSource("mine", "manifest.json"); err == nil {
		t.Error("manifest.json 不应可读")
	}
	if _, err := m.ReadLocalSource("market", "main.lua"); err == nil {
		t.Error("非自建插件应拒绝")
	}
}

// TestWriteLocalSourceGuards 非 main.lua / 非自建 / 改名 三个错误分支（均在 Start 前 return）。
func TestWriteLocalSourceGuards(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dir: root}
	mkLocal(t, root, "mine", `{"name":"mine","runtime":"lua"}`, "return {}")
	ctx := t.Context()

	if err := m.WriteLocalSource(ctx, "mine", "manifest.json", "{}"); err == nil {
		t.Error("非 main.lua 应拒绝")
	}
	if err := m.WriteLocalSource(ctx, "market", "main.lua", "return {}"); err == nil {
		t.Error("非自建插件应拒绝")
	}
	// 代码侧 PLUGIN_NAME 与身份不一致 → 改名拒绝
	renamed := `local PLUGIN_NAME = "other"
return {}`
	if err := m.WriteLocalSource(ctx, "mine", "main.lua", renamed); err == nil {
		t.Error("改名应被拒绝")
	}
}

// TestSyncLocalLabel 代码侧 LABEL_ZH/EN 变更同步进 manifest.json；EN 空不覆盖。
func TestSyncLocalLabel(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dir: root}
	mkLocal(t, root, "mine", `{"name":"mine","runtime":"lua","label":{"zh":"旧","en":"Old"}}`, "return {}")
	dir := filepath.Join(root, LocalNamespace, "mine")

	m.syncLocalLabel(dir, "mine", `local PLUGIN_LABEL_ZH = "新"
local PLUGIN_LABEL_EN = ""`)

	data, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	var mf PackageManifest
	json.Unmarshal(data, &mf)
	if mf.Label["zh"] != "新" {
		t.Errorf("zh 应更新为 新，got %q", mf.Label["zh"])
	}
	if mf.Label["en"] != "Old" {
		t.Errorf("EN 为空时不应覆盖，got %q", mf.Label["en"])
	}
}
