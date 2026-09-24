// vm.go — VM 生命周期与复用池。newVM 建一个沙箱化的 LState，执行 main.lua 并捕获其
// 返回的 module table（约定：脚本 `local M={}; ...; return M`）；约定函数是 M 的字段。
// vmPool 复用 VM，避免每次 RPC 重编脚本。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
)

// luaVM 一个已加载脚本的 VM：LState + 脚本返回的 module table。
type luaVM struct {
	L   *lua.LState
	mod *lua.LTable // main.lua `return M` 的 M
}

// fn 取 module 上的约定函数（不存在返回 nil）。
func (v *luaVM) fn(name string) lua.LValue {
	if v.mod == nil {
		return lua.LNil
	}
	return v.L.GetField(v.mod, name)
}

// newVM 创建沙箱 VM，执行 main.lua 并捕获返回的 module table。
func newVM(dir string, host *sdk.Host) (*luaVM, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	openSafeLibs(L)
	stripDangerous(L)
	registerCPH(L, host, dir)
	installSafeRequire(L, dir)

	src, err := os.ReadFile(filepath.Join(dir, "main.lua"))
	if err != nil {
		L.Close()
		return nil, fmt.Errorf("read main.lua: %w", err)
	}
	chunk, err := L.LoadString(string(src))
	if err != nil {
		L.Close()
		return nil, fmt.Errorf("compile main.lua: %w", err)
	}
	L.Push(chunk)
	if err := L.PCall(0, 1, nil); err != nil { // 执行脚本，取 1 个返回值
		L.Close()
		return nil, fmt.Errorf("run main.lua: %w", err)
	}
	ret := L.Get(-1)
	L.Pop(1)
	mod, ok := ret.(*lua.LTable)
	if !ok {
		L.Close()
		return nil, fmt.Errorf("main.lua must `return M` (a table), got %s", ret.Type())
	}
	return &luaVM{L: L, mod: mod}, nil
}

// vmPool 复用已加载脚本的 VM。空闲时懒建；用完放回。
type vmPool struct {
	dir  string
	host *sdk.Host
	mu   sync.Mutex
	free []*luaVM
}

func newVMPool(dir string, host *sdk.Host) *vmPool {
	return &vmPool{dir: dir, host: host}
}

func (p *vmPool) get() (*luaVM, error) {
	p.mu.Lock()
	if n := len(p.free); n > 0 {
		v := p.free[n-1]
		p.free = p.free[:n-1]
		p.mu.Unlock()
		return v, nil
	}
	p.mu.Unlock()
	return newVM(p.dir, p.host)
}

func (p *vmPool) put(v *luaVM) {
	p.mu.Lock()
	p.free = append(p.free, v)
	p.mu.Unlock()
}
