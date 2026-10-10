// main.go — luahost：一个用 sdk 写的通用 Go 插件，把 ClawPlugin 契约的每个 RPC 翻成 Lua 调用。
// 核心眼里它就是普通 go-plugin 插件；差异全在方法体（见 rpc.go）：proto ↔ table，转调 main.lua。
package luahost

import (
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// New 由平台入口指定脚本工作区，桌面和 Android 共用同一运行时。
func New(dir string) *luahost { return &luahost{dir: dir} }
func (h *luahost) Close() error {
	if h.pool != nil {
		h.pool.close()
	}
	return nil
}

// luahost 实现 sdk.Plugin（pb.ClawPluginServer）。未实现的 RPC 由内嵌 Unimplemented 降级。
type luahost struct {
	pb.UnimplementedClawPluginServer
	dir  string
	host *sdk.Host
	pool *vmPool
}

// SetHost 实现 sdk.HostAware：拿到宿主回调后建 VM 池（cph.log 等据此反向调核心）。
func (h *luahost) SetHost(host *sdk.Host) {
	h.host = host
	h.pool = newVMPool(h.dir, host)
}
