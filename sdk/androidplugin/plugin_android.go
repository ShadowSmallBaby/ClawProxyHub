// Package androidplugin 为生成的 JNI 入口接管连接并管理插件会话。
package androidplugin

import (
	"fmt"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/platform/androidconn"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
)

var sessions androidconn.Registry

// Open 接管两个文件描述符；平台必须先完成调用身份校验。
func Open(requestFD, callbackFD int, impl sdk.Plugin) (int64, error) {
	requests, err := androidconn.Take(requestFD)
	callbacks, callbackErr := androidconn.Take(callbackFD)
	if err != nil || callbackErr != nil {
		if requests != nil {
			requests.Close()
		}
		if callbacks != nil {
			callbacks.Close()
		}
		return 0, fmt.Errorf("plugin socket: %v, %v", err, callbackErr)
	}
	session, err := transport.ServePlugin(requests, callbacks, impl)
	if err != nil {
		return 0, err
	}
	return sessions.Add(session), nil
}

func Close(id int64) { sessions.Close(id) }
