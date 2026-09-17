// Package plugin — 插件管理器：子进程生命周期、gRPC 握手、目录扫描。
// 采用 hashicorp/go-plugin：插件是独立二进制，经 stdout 握手 + gRPC on localhost 通信。
package plugin

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// hostBrokerID 宿主 ClawHost 服务的 broker 通道号（与 sdk.HostBrokerID 一致）。
const hostBrokerID = sdk.HostBrokerID

// CoreVersion 核心版本（构建时注入，暂为常量）。
const CoreVersion = "0.1.0"

// Manager 持有全部已启动的插件实例。
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]*Instance // key: plugin name
	dir     string
	host    *HostService
	catalog map[string]string // model id → plugin name
}

// Instance 一个运行中的插件子进程。
type Instance struct {
	Name     string
	Manifest *pb.Manifest
	client   *goplugin.Client
	rpc      pb.ClawPluginClient
}

// ClawPluginPlugin 实现 goplugin.Plugin，把 gRPC 服务暴露给 go-plugin 框架。
type ClawPluginPlugin struct {
	goplugin.Plugin
	host *HostService
}

// GRPCServer 核心进程不作为插件运行，此路不走。
func (p *ClawPluginPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	return fmt.Errorf("core does not run as a plugin")
}

// GRPCClient 核心侧拿到插件客户端桩，同时挂出宿主回调服务。
func (p *ClawPluginPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	if p.host != nil {
		// AcceptAndServe 阻塞等待插件反连，必须异步
		go p.host.ServeHost(broker)
	}
	return pb.NewClawPluginClient(c), nil
}

// handshakeConfig go-plugin 进程握手配置。
var handshakeConfig = sdk.HandshakeConfig()

// pluginSet 核心侧声明可对接的插件接口。
var pluginSet = goplugin.PluginSet{
	"claw_plugin": &ClawPluginPlugin{},
}

// NewManager 创建插件管理器。
func NewManager(dir string, db *gorm.DB) *Manager {
	return &Manager{
		plugins: make(map[string]*Instance),
		dir:     dir,
		host:    NewHostService(db),
		catalog: map[string]string{},
	}
}

// RefreshCatalog 从各插件拉取模型目录（无需凭据的部分）。
func (m *Manager) RefreshCatalog(ctx context.Context) {
	catalog := map[string]string{}
	m.mu.RLock()
	plugins := make([]*Instance, 0, len(m.plugins))
	for _, inst := range m.plugins {
		plugins = append(plugins, inst)
	}
	m.mu.RUnlock()

	for _, inst := range plugins {
		ml, err := inst.rpc.ListModels(ctx, &pb.CredentialBlob{})
		if err != nil {
			continue
		}
		for _, mo := range ml.Models {
			catalog[mo.Id] = inst.Name
		}
	}
	m.mu.Lock()
	m.catalog = catalog
	m.mu.Unlock()
}

// Models 返回聚合模型目录。
func (m *Manager) Models() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.catalog))
	for k, v := range m.catalog {
		out[k] = v
	}
	return out
}

// Scan 扫描插件目录，返回可启动的二进制路径列表。
// 插件目录布局：<dir>/<name>/plugin-<os>-<arch>[.exe] + manifest.json（+ 图标）
func (m *Manager) Scan() ([]string, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugin dir: %w", err)
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		bin, err := pluginBinary(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue // 目录不完整，跳过
		}
		found = append(found, bin)
	}
	return found, nil
}

// pluginBinary 定位插件目录下匹配当前平台的二进制。
func pluginBinary(dir string) (string, error) {
	name := filepath.Base(dir)
	candidates := []string{
		filepath.Join(dir, fmt.Sprintf("plugin-%s-%s", runtimeOS(), runtimeArch())),
	}
	if runtimeOS() == "windows" {
		candidates = append(candidates, candidates[0]+".exe")
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("no binary for %s/%s in %s", runtimeOS(), runtimeArch(), name)
}

// Start 启动一个插件子进程并完成契约握手。
func (m *Manager) Start(ctx context.Context, binPath string) (*Instance, error) {
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: handshakeConfig,
		Plugins: goplugin.PluginSet{
			// 每个插件实例独立持有宿主服务，便于按插件隔离状态
			"claw_plugin": &ClawPluginPlugin{host: m.host},
		},
		Cmd:              execCommand(binPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("connect plugin %s: %w", binPath, err)
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

	// 契约握手：协议版本不一致直接拒载
	hs, err := pc.Handshake(ctx, &pb.HandshakeRequest{
		CoreVersion:     CoreVersion,
		ProtocolVersion: sdk.ProtocolVersion,
	})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("handshake: %w", err)
	}
	if hs.Error != nil && hs.Error.Code != 0 {
		client.Kill()
		return nil, fmt.Errorf("plugin rejected: %s", hs.Error.Message)
	}
	if hs.Manifest == nil || hs.Manifest.ProtocolVersion != sdk.ProtocolVersion {
		client.Kill()
		return nil, fmt.Errorf("protocol version mismatch: core=%d plugin=%d",
			sdk.ProtocolVersion, hs.Manifest.GetProtocolVersion())
	}

	inst := &Instance{Name: hs.Manifest.Name, Manifest: hs.Manifest, client: client, rpc: pc}
	m.mu.Lock()
	m.plugins[inst.Name] = inst
	m.mu.Unlock()
	return inst, nil
}

// Get 按名称取运行中的插件实例；进程已崩溃时自动重启。
func (m *Manager) Get(name string) (*Instance, bool) {
	m.mu.RLock()
	inst, ok := m.plugins[name]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !inst.client.Exited() {
		return inst, true
	}
	// 崩溃残留：清掉死实例并尝试从插件目录重启
	m.mu.Lock()
	delete(m.plugins, name)
	m.mu.Unlock()
	bin, err := pluginBinary(filepath.Join(m.dir, name))
	if err != nil {
		return nil, false
	}
	fmt.Printf("[plugin] %s crashed, restarting\n", name)
	if inst2, err := m.Start(context.Background(), bin); err == nil {
		return inst2, true
	}
	fmt.Printf("[plugin] restart %s failed: %v\n", name, err)
	return nil, false
}

// Names 运行中的插件名列表。
func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.plugins))
	for n := range m.plugins {
		names = append(names, n)
	}
	return names
}

// Endpoints 插件声明的对外端点方言（空 = 全支持）。
func (m *Manager) Endpoints(name string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if inst, ok := m.plugins[name]; ok && inst.Manifest != nil {
		return inst.Manifest.Endpoints
	}
	return nil
}

// Client 返回插件的 gRPC 客户端。
func (i *Instance) Client() pb.ClawPluginClient { return i.rpc }

// Chat 实现 gateway.PluginRegistry：按插件路由并泵出事件流。
// pluginName 为空时按模型目录解析（非路由直连场景）。
func (m *Manager) Chat(req *pb.ChatRequest, pluginName string, cred *pb.CredentialBlob) (chan *pb.StreamEvent, error) {
	if pluginName == "" {
		var ok bool
		pluginName, ok = m.ResolveModel(req.Model)
		if !ok {
			return nil, fmt.Errorf("model %q not found", req.Model)
		}
	}
	inst, ok := m.Get(pluginName) // 含崩溃自愈
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}

	req.Credential = cred

	stream, err := inst.rpc.Chat(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("plugin chat: %w", err)
	}

	events := make(chan *pb.StreamEvent, 32)
	go func() {
		defer close(events)
		for {
			ev, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					events <- &pb.StreamEvent{Event: &pb.StreamEvent_TaskFailed{
						TaskFailed: &pb.TaskFailed{Error: &pb.Error{
							Code: 1, Message: err.Error(),
						}},
					}}
				}
				return
			}
			events <- ev
		}
	}()
	return events, nil
}

// Stop 停止一个插件（供卸载/升级/停用）。
func (m *Manager) Stop(name string) {
	m.mu.Lock()
	inst, ok := m.plugins[name]
	if ok {
		delete(m.plugins, name)
	}
	m.mu.Unlock()
	if ok {
		inst.client.Kill()
	}
}

// ResolveModel 模型名 → 插件名（gateway.PluginRegistry 实现）。
func (m *Manager) ResolveModel(model string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	name, ok := m.catalog[model]
	return name, ok
}

// StopAll 停止全部插件（进程退出前调用）。
func (m *Manager) StopAll() {
	m.mu.Lock()
	names := make([]string, 0, len(m.plugins))
	for n := range m.plugins {
		names = append(names, n)
	}
	m.mu.Unlock()
	for _, n := range names {
		m.Stop(n)
	}
}
