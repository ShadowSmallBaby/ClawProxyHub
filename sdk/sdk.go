// Package sdk — 插件开发工具包：实现 ClawPluginServer 即可接入核心。
package sdk

import (
	"context"
	"fmt"
	"os"
	"sync"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// 与核心约定的常量。核心接受 [MinProtocolVersion, ProtocolVersion] 区间内的插件（go-plugin 协商），
// 插件侧只实现 ProtocolVersion 一个版本。
// v2：实例层（CredentialBlob/LoginRequest/GetSettingsRequest 带 instance_id，Manifest.instance_schema）。
const (
	ProtocolVersion    int32 = 2
	MinProtocolVersion int32 = 1
	// CapabilityInstances 插件声明支持多实例（同一插件挂多个站点/部署）；未声明的插件只有一个默认实例。
	CapabilityInstances string = "instances"
	// ExtraClientUserAgent ChatRequest.extra 键：对话请求应使用的 User-Agent（核心按 路由 UA > 全局 UA > 客户端 UA 解析后注入；插件按需透传上游）。
	ExtraClientUserAgent string = "client_user_agent"
	// SettingBrowserUserAgent 宿主 GetSettings 合并视图的保留键：全局浏览器 UA（空 / 缺失 = 插件用内置值）。
	SettingBrowserUserAgent string = "_browser_user_agent"
	MagicCookieKey       string = "CPH_PLUGIN"
	MagicCookieVal       string = "claw-proxy-hub-plugin"
	// HostBrokerID 宿主 ClawHost 服务在 broker 上的固定通道号。
	HostBrokerID uint32 = 1000
)

// HandshakeConfig go-plugin 进程握手配置。
func HandshakeConfig() goplugin.HandshakeConfig {
	return goplugin.HandshakeConfig{
		ProtocolVersion:  uint(ProtocolVersion),
		MagicCookieKey:   MagicCookieKey,
		MagicCookieValue: MagicCookieVal,
	}
}

// Host 宿主回调能力（由核心注入，插件实现里可取用）。
// 连接在注入时即异步建立（broker 连接信息仅短暂有效）；失败后下次调用会重试。
type Host struct {
	dial   func() (pb.ClawHostClient, error)
	mu     sync.Mutex
	client pb.ClawHostClient
}

func (h *Host) conn() pb.ClawHostClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.client != nil {
		return h.client
	}
	c, err := h.dial()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[cph-sdk] host dial failed: %v\n", err)
		return nil
	}
	h.client = c
	return c
}

// Log 写统一日志管道。
func (h *Host) Log(level, message string) {
	if c := h.conn(); c != nil {
		c.Log(context.Background(), &pb.LogEntry{Level: level, Message: message})
	}
}

// StoreGet 读插件状态。
func (h *Host) StoreGet(key string) ([]byte, bool) {
	c := h.conn()
	if c == nil {
		return nil, false
	}
	resp, err := c.StoreGet(context.Background(), &pb.StoreGetRequest{Key: key})
	if err != nil || !resp.Found {
		return nil, false
	}
	return resp.Value, true
}

// StorePut 写插件状态。
func (h *Host) StorePut(key string, value []byte) {
	if c := h.conn(); c != nil {
		c.StorePut(context.Background(), &pb.StorePutRequest{Key: key, Value: value})
	}
}

// Settings 读插件设置（核心管理界面在线编辑；pluginName 为本插件 id）。
// 返回原始 JSON（结构由 manifest.settings_schema 定义），读取失败回 nil 由调用方用默认值。
func (h *Host) Settings(pluginName string) []byte {
	return h.InstanceSettings(pluginName, 0)
}

// InstanceSettings 读实例视图的设置：插件设置 ← 实例设置 ← {"base_url": 实例地址}。
// instanceID 为 0 时等价于 Settings（仅插件级）。
func (h *Host) InstanceSettings(pluginName string, instanceID int64) []byte {
	c := h.conn()
	if c == nil {
		return nil
	}
	resp, err := c.GetSettings(context.Background(), &pb.GetSettingsRequest{Plugin: pluginName, InstanceId: instanceID})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[cph-sdk] GetSettings failed: %v\n", err)
		return nil
	}
	return resp.Values
}

// Plugin 插件作者需要实现的全部：gRPC 服务 + 宿主注入点。
type Plugin interface {
	pb.ClawPluginServer
}

// HostAware 可选：实现后核心会把宿主回调注入插件。
type HostAware interface {
	SetHost(host *Host)
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
			return grpc.NewServer(opts...)
		},
	}
	goplugin.Serve(opts)
}
