package main

import (
	"flag"
	luahost "github.com/ShadowSmallBaby/ClawProxyHub/hosts/luahost"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", "", "插件工作区目录")
	flag.Parse()
	if *dir == "" {
		exe, err := os.Executable()
		if err != nil {
			exe = os.Args[0]
		}
		*dir = filepath.Dir(exe)
	}
	sdk.Serve(luahost.New(*dir))
}
