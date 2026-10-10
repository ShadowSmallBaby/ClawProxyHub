// 最小 Android 测试插件只声明握手，用于验证当前核心 SDK 与宿主加载器。
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/androidplugin"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

type testPlugin struct {
	pb.UnimplementedClawPluginServer
}

func (*testPlugin) Handshake(_ context.Context, request *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	if request.ProtocolVersion != sdk.ProtocolVersion {
		return &pb.HandshakeResponse{Error: &pb.Error{Code: 1, Message: "protocol mismatch"}}, nil
	}
	return &pb.HandshakeResponse{Manifest: &pb.Manifest{
		Name: "androidtest", Version: "0.0.0", Author: "cph",
		Label: map[string]string{"en": "Android test"}, ProtocolVersion: sdk.ProtocolVersion,
	}}, nil
}

//export CPHInitializePlugin
func CPHInitializePlugin(directory *C.char) C.int {
	path := C.GoString(directory)
	if err := os.MkdirAll(path, 0700); err != nil {
		return 0
	}
	if err := os.Setenv("TMPDIR", path); err != nil {
		return 0
	}
	stall("stall-open")
	return 1
}

//export CPHOpenPlugin
func CPHOpenPlugin(requestFD, callbackFD C.int) C.longlong {
	id, err := androidplugin.Open(int(requestFD), int(callbackFD), &testPlugin{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 0
	}
	return C.longlong(id)
}

//export CPHClosePlugin
func CPHClosePlugin(id C.longlong) {
	stall("stall-close")
	androidplugin.Close(int64(id))
}

// 仅测试库通过标记文件模拟无法响应取消的原生调用。
func stall(marker string) {
	if _, err := os.Stat(filepath.Join(os.Getenv("TMPDIR"), marker)); err == nil {
		for {
			time.Sleep(time.Second)
		}
	}
}

func main() {}
