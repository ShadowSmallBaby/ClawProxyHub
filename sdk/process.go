//go:build !android

// 桌面子进程入口独立编译，Android 通过平台会话启动服务。
package sdk

import (
	"context"
	"fmt"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// HandshakeConfig go-plugin 进程握手配置。
func HandshakeConfig() goplugin.HandshakeConfig {
	return goplugin.HandshakeConfig{
		ProtocolVersion:  uint(ProtocolVersion),
		MagicCookieKey:   MagicCookieKey,
		MagicCookieValue: MagicCookieVal,
	}
}

// pluginServer 包装用户实现，注册进 gRPC 并接通宿主回调。
type pluginServer struct {
	goplugin.NetRPCUnsupportedPlugin
	impl Plugin
}

func (s *pluginServer) GRPCServer(broker *goplugin.GRPCBroker, srv *grpc.Server) error {
	if ha, ok := s.impl.(HostAware); ok {
		host := &Host{dial: func() (pb.ClawHostClient, error) {
			conn, err := broker.Dial(HostBrokerID)
			if err != nil {
				return nil, err
			}
			return pb.NewClawHostClient(conn), nil
		}}
		ha.SetHost(host)
		// 宿主 Accept 发来的连接信息 broker 只保留 5s，必须在握手期内建连；
		// 之后复用同一 gRPC 连接（底层自动重连），懒到首次调用会超时拿不到。
		go host.conn()
	}
	pb.RegisterClawPluginServer(srv, s.impl)
	return nil
}

// GRPCClient 插件进程不作为客户端使用，仅为满足 goplugin.GRPCPlugin。
func (s *pluginServer) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, fmt.Errorf("plugin does not act as a grpc client")
}

// Serve 启动插件进程，阻塞至核心将其关闭。
// 插件二进制的 main 只需一行：sdk.Serve(impl)。
func Serve(impl Plugin) {
	opts := &goplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig(),
		Plugins: goplugin.PluginSet{
			"claw_plugin": &pluginServer{impl: impl},
		},
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			max := GRPCMaxMsgSize()
			opts = append(opts, grpc.MaxRecvMsgSize(max), grpc.MaxSendMsgSize(max))
			return grpc.NewServer(opts...)
		},
	}
	goplugin.Serve(opts)
}
