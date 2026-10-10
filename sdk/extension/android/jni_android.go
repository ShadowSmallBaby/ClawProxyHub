package android

/*
#include <stdint.h>
*/
import "C"
import (
	"os"
	"sync"

	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

var registration struct {
	sync.Mutex
	factory func() ext.Service
}

// Register 在 Android main 包初始化时注册业务处理器，每个 worker 只加载一个扩展。
func Register(factory func() ext.Service) {
	registration.Lock()
	defer registration.Unlock()
	if factory == nil || registration.factory != nil {
		panic("Android extension requires exactly one service factory")
	}
	registration.factory = factory
}

//export CPHExtensionOpen
func CPHExtensionOpen(fd C.int) C.longlong {
	registration.Lock()
	factory := registration.factory
	registration.Unlock()
	if factory == nil {
		if fd >= 0 {
			_ = os.NewFile(uintptr(fd), "extension-socket").Close()
		}
		return 0
	}
	id, err := Open(int(fd), factory())
	if err != nil {
		return 0
	}
	return C.longlong(id)
}

//export CPHExtensionClose
func CPHExtensionClose(id C.longlong) { Close(int64(id)) }
