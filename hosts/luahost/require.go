// require.go — 沙箱化 require：仅允许 require("lib.<name>") 加载插件目录下 lib/*.lua，带模块缓存、禁路径逃逸。
// 子模块在同一沙箱 L 里执行，继承白名单标准库与 cph.*；可互相 require。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// installSafeRequire 注入受限 require：段名仅限 [A-Za-z0-9_]，"lib." 前缀，映射 <dir>/lib/<path>.lua。
func installSafeRequire(L *lua.LState, dir string) {
	loaded := L.NewTable() // 模块缓存：模块名 → 返回值（与标准 require 语义一致，只执行一次）
	L.SetGlobal("require", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		if cached := L.GetField(loaded, name); cached != lua.LNil {
			L.Push(cached)
			return 1
		}
		rel, err := libPath(name)
		if err != nil {
			L.RaiseError("require: %v", err)
		}
		src, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			L.RaiseError("require %q: %v", name, err)
		}
		fn, err := L.LoadString(string(src))
		if err != nil {
			L.RaiseError("require %q: %v", name, err)
		}
		L.Push(fn)
		L.Call(0, 1) // 执行子模块，取 1 个返回值（出错会 raise，由外层 PCall 兜住）
		ret := L.Get(-1)
		if ret == lua.LNil { // 无返回值的模块缓存为 true（同标准 require）
			ret = lua.LTrue
			L.Pop(1)
			L.Push(ret)
		}
		L.SetField(loaded, name, ret)
		return 1
	}))
}

// libPath 校验模块名并映射为相对路径：必须 "lib." 前缀，段仅限 [A-Za-z0-9_]，天然禁 .. 与斜杠逃逸。
func libPath(name string) (string, error) {
	if !strings.HasPrefix(name, "lib.") {
		return "", fmt.Errorf("only require(\"lib.*\") allowed, got %q", name)
	}
	segs := strings.Split(name, ".")
	for _, s := range segs {
		if s == "" || !validSeg(s) {
			return "", fmt.Errorf("invalid module name %q", name)
		}
	}
	return filepath.Join(segs...) + ".lua", nil // lib.sub.foo → lib/sub/foo.lua
}

func validSeg(s string) bool {
	for _, r := range s {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
