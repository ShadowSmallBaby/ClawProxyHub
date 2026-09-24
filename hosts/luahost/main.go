// main.go — luahost：一个用 sdk 写的通用 Go 插件，把 ClawPlugin 契约的每个 RPC 翻成 Lua 调用。
// 核心眼里它就是普通 go-plugin 插件；差异全在方法体（见 rpc.go）：proto ↔ table，转调 main.lua。
package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func main() {
	// --dir 指定插件目录（P1 共享 luahost：核心以 `luahost --dir <插件目录>` 启动）；
	// 未指定则回退可执行文件所在目录（独立二进制/旧式逐插件拷贝仍兼容）。
	dir := flag.String("dir", "", "插件目录（含 main.lua）")
	flag.Parse()
	d := *dir
	if d == "" {
		d = exeDir()
	}
	sdk.Serve(&luahost{dir: d})
}

// exeDir 插件目录：main.lua 与 plugin-<os>-<arch> 同级。
func exeDir() string {
	if p, err := os.Executable(); err == nil {
		return filepath.Dir(p)
	}
	return filepath.Dir(os.Args[0])
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
