//go:build !android

package plugin

import (
	"context"
	"fmt"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// ClawPluginPlugin 实现 goplugin.Plugin，把 gRPC 服务暴露给 go-plugin 框架。
type ClawPluginPlugin struct {
	goplugin.Plugin
	host pb.ClawHostServer
}

// GRPCServer 核心进程不作为插件运行，此路不走。
func (p *ClawPluginPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	return fmt.Errorf("core does not run as a plugin")
}

// GRPCClient 核心侧拿到插件客户端桩，同时挂出宿主回调服务。
func (p *ClawPluginPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	if p.host != nil {
		// AcceptAndServe 阻塞等待插件反连，必须异步
		go broker.AcceptAndServe(sdk.HostBrokerID, func(opts []grpc.ServerOption) *grpc.Server {
			max := sdk.GRPCMaxMsgSize()
			opts = append(opts, grpc.MaxRecvMsgSize(max), grpc.MaxSendMsgSize(max))
			server := grpc.NewServer(opts...)
			pb.RegisterClawHostServer(server, p.host)
			return server
		})
	}
	return pb.NewClawPluginClient(c), nil
}

// handshakeConfig go-plugin 进程握手配置。
var handshakeConfig = sdk.HandshakeConfig()

type processRuntime struct{ manager *Manager }

func defaultRuntime(m *Manager) Runtime             { return &processRuntime{manager: m} }
func (r *processRuntime) Available(dir string) bool { return r.manager.launchable(dir) }
func (r *processRuntime) Start(ctx context.Context, dir string, host pb.ClawHostServer) (Session, error) {
	_, cmd, err := r.manager.resolveLaunch(dir)
	if err != nil {
		return nil, err
	}
	// 每个插件实例独立持有宿主服务（forPlugin 按插件名隔离 store 等状态）
	set := goplugin.PluginSet{"claw_plugin": &ClawPluginPlugin{host: host}}
	versioned := map[int]goplugin.PluginSet{}
	for v := sdk.MinProtocolVersion; v <= sdk.ProtocolVersion; v++ {
		versioned[int(v)] = set
	}
	maxMsg := sdk.GRPCMaxMsgSize()
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  handshakeConfig,
		VersionedPlugins: versioned,
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(maxMsg),
				grpc.MaxCallSendMsgSize(maxMsg),
			),
		},
	})

	finished := make(chan struct{})
	watcherDone := make(chan struct{})
	defer func() { close(finished); <-watcherDone }()
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			client.Kill()
		case <-finished:
		}
	}()
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("connect plugin %s: %w", dir, err)
	}

	raw, err := rpcClient.Dispense("claw_plugin")
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispense claw_plugin: %w", err)
	}

	pc, ok := raw.(pb.ClawPluginClient)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("unexpected plugin client type %T", raw)
	}

	return &processSession{process: client, rpc: pc}, nil
}

type processSession struct {
	process *goplugin.Client
	rpc     pb.ClawPluginClient
}

func (s *processSession) Client() pb.ClawPluginClient { return s.rpc }
func (s *processSession) ProtocolVersion() int32      { return int32(s.process.NegotiatedVersion()) }
func (s *processSession) Exited() bool                { return s.process.Exited() }
func (s *processSession) Close() error                { s.process.Kill(); return nil }
