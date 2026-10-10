// 桌面入口只连接宿主提供的 RPC 管道。
package main

import (
	"log"

	editor "github.com/ShadowSmallBaby/ClawProxyHub/extensions/lua-editor/backend"
	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func main() {
	if err := ext.Serve(editor.Service()); err != nil {
		log.Fatal(err)
	}
}
