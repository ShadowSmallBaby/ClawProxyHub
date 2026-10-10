// Android 入口复用业务处理器，JNI 和连接生命周期交给扩展 SDK。
package main

import (
	editor "github.com/ShadowSmallBaby/ClawProxyHub/extensions/lua-editor/backend"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension/android"
)

func init() { android.Register(editor.Service) }
func main() {}
