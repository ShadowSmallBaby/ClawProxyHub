//go:build !android

package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// 子进程使用历史的根 main + go-plugin 形式，不依赖业务工厂或 Service 接口。
func TestMain(m *testing.M) {
	if value := os.Getenv("CPH_TEST_PROCESS_PROTOCOL"); value != "" {
		protocol, err := strconv.Atoi(value)
		if err != nil {
			os.Exit(2)
		}
		config := sdk.HandshakeConfig()
		config.ProtocolVersion = uint(protocol)
		goplugin.Serve(&goplugin.ServeConfig{
			HandshakeConfig: config,
			Plugins:         goplugin.PluginSet{"claw_plugin": &legacyTestServer{protocol: int32(protocol)}},
			GRPCServer:      goplugin.DefaultGRPCServer,
		})
		return
	}
	os.Exit(m.Run())
}

type legacyTestServer struct {
	goplugin.NetRPCUnsupportedPlugin
	protocol int32
}

func (s *legacyTestServer) GRPCServer(_ *goplugin.GRPCBroker, server *grpc.Server) error {
	pb.RegisterClawPluginServer(server, &sessionPlugin{name: "test", protocol: s.protocol})
	return nil
}
func (*legacyTestServer) GRPCClient(context.Context, *goplugin.GRPCBroker, *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

func TestProcessRuntimeProtocolCompatibility(t *testing.T) {
	for _, protocol := range []int32{sdk.MinProtocolVersion, sdk.ProtocolVersion} {
		t.Run(strconv.Itoa(int(protocol)), func(t *testing.T) {
			t.Setenv("CPH_TEST_PROCESS_PROTOCOL", strconv.Itoa(int(protocol)))
			root, dir := pluginTestDir(t, "test")
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			name := "plugin-" + runtimeOS() + "-" + runtimeArch()
			if runtimeOS() == "windows" {
				name += ".exe"
			}
			data, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0755); err != nil {
				t.Fatal(err)
			}
			m := NewManager(root, nil)
			t.Cleanup(m.StopAll)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			inst, err := m.Start(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			if inst.Protocol != protocol {
				t.Fatalf("negotiated %d, want %d", inst.Protocol, protocol)
			}
			response, err := inst.Client().RunTask(ctx, &pb.RunTaskRequest{})
			if err != nil || response.GetSummary() != "service-task" {
				t.Fatalf("legacy task: %v %v", response, err)
			}
		})
	}
}

// 使用旧版发布产物验证桌面插件的二进制兼容性。
func TestExistingDesktopBinary(t *testing.T) {
	dir := os.Getenv("CPH_TEST_EXISTING_PLUGIN_DIR")
	if dir == "" {
		t.Skip("set CPH_TEST_EXISTING_PLUGIN_DIR to an existing plugin directory")
	}
	m := NewManager(filepath.Dir(dir), nil)
	defer m.StopAll()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inst, err := m.Start(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Manifest.GetName() != filepath.Base(dir) {
		t.Fatal("wrong existing binary identity")
	}
}
