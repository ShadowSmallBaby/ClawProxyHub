// vm.go — VM 生命周期与复用池。newVM 建一个沙箱化的 LState，执行 main.lua 并捕获其
// 返回的 module table（约定：脚本 `local M={}; ...; return M`）；约定函数是 M 的字段。
// vmPool 复用 VM，避免每次 RPC 重编脚本。
package luahost

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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
func newVM(dir string, host *sdk.Host, contexts ...context.Context) (*luaVM, error) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	L := lua.NewState(lua.Options{SkipOpenLibs: true, CallStackSize: 256, RegistrySize: 1024, RegistryMaxSize: 32768})
	L.SetContext(initCtx)
	openSafeLibs(L)
	stripDangerous(L)
	registerCPH(L, host, dir)
	installSafeRequire(L, dir)

	info, err := os.Stat(filepath.Join(dir, "main.lua"))
	if err != nil || info.Size() > 4<<20 {
		L.Close()
		return nil, fmt.Errorf("main.lua missing or exceeds 4 MiB")
	}
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
	L.SetContext(ctx)
	return &luaVM{L: L, mod: mod}, nil
}

// vmPool 复用已加载脚本的 VM。空闲时懒建；用完放回。
type vmPool struct {
	closed bool
	dir    string
	host   *sdk.Host
	mu     sync.Mutex
	free   []*luaVM
	slots  chan struct{}
}

func newVMPool(dir string, host *sdk.Host) *vmPool {
	return &vmPool{dir: dir, host: host, slots: make(chan struct{}, 8)}
}

func (p *vmPool) get(contexts ...context.Context) (*luaVM, error) {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		<-p.slots
		return nil, fmt.Errorf("Lua runtime closed")
	}
	if n := len(p.free); n > 0 {
		v := p.free[n-1]
		p.free = p.free[:n-1]
		p.mu.Unlock()
		v.L.SetContext(ctx)
		return v, nil
	}
	p.mu.Unlock()
	v, err := newVM(p.dir, p.host, ctx)
	if err != nil {
		<-p.slots
	}
	return v, err
}

func (p *vmPool) put(v *luaVM) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		v.L.Close()
		<-p.slots
		return
	}
	v.L.RemoveContext()
	v.L.SetTop(0)
	p.free = append(p.free, v)
	p.mu.Unlock()
	<-p.slots
}

func (p *vmPool) discard(v *luaVM) { v.L.Close(); <-p.slots }

func (p *vmPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, v := range p.free {
		v.L.Close()
	}
	p.free = nil
}
