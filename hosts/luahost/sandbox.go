// sandbox.go — Lua 沙箱：只装白名单标准库、剔除可越权/触达宿主的符号。
// 一切出站能力只经 cph.*（见 cph.go），脚本无法自建 socket、读环境变量或触达文件系统。
package main

import lua "github.com/yuin/gopher-lua"

// openSafeLibs 只装白名单标准库：base / table / string / math。
// 不装 os / io / debug / package（禁文件、进程、反射、动态加载）。
func openSafeLibs(L *lua.LState) {
	for _, lib := range []struct {
		name string
		open lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(lib.open))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}
}

// stripDangerous 从 base 移除动态加载与进程级符号。require 不在此——由 installSafeRequire 换成
// 只认 lib.* 的白名单版。print 会写 stdout（go-plugin 握手管道），一并移除——脚本用 cph.log 输出。
func stripDangerous(L *lua.LState) {
	for _, name := range []string{
		"dofile", "loadfile", "load", "loadstring",
		"collectgarbage", "print", "module",
	} {
		L.SetGlobal(name, lua.LNil)
	}
}
