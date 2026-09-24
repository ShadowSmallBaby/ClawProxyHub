// Package plugin — 插件管理器：子进程生命周期、gRPC 握手、目录扫描。
// 采用 hashicorp/go-plugin：插件是独立二进制，经 stdout 握手 + gRPC on localhost 通信。
package plugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/fingerprint"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/runlog"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/version"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// hostBrokerID 宿主 ClawHost 服务的 broker 通道号（与 sdk.HostBrokerID 一致）。
const hostBrokerID = sdk.HostBrokerID

// CoreVersion 核心版本（握手时告知插件，供 min_core_version 校验）。
var CoreVersion = version.Core

// Manager 持有全部已启动的插件实例。
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]*Instance // key: plugin name
	dir     string
	db      *gorm.DB // plugins 表记录同步（nil = 不落库，测试用）
	host    *HostService
	runLog  *runlog.Logger
	catalog map[string]string // model id → plugin name
}

// Instance 一个运行中的插件子进程。
type Instance struct {
	Name     string
	Manifest *pb.Manifest
	Protocol int32 // 协商到的契约版本（旧插件为 1：无实例维度，字段被忽略）
	client   *goplugin.Client
	rpc      pb.ClawPluginClient
}

// MultiInstance 插件是否支持多实例：契约 ≥2 且声明 instances 能力；否则只有默认实例。
func (i *Instance) MultiInstance() bool { return ManifestMultiInstance(i.Manifest, i.Protocol) }

// ManifestMultiInstance 按 manifest + 契约版本判定多实例能力（停止的插件用 DB 快照判定时复用）。
func ManifestMultiInstance(m *pb.Manifest, protocol int32) bool {
	if protocol < 2 || m == nil {
		return false
	}
	for _, c := range m.Capabilities {
		if c == sdk.CapabilityInstances {
			return true
		}
	}
	return false
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

// NewManager 创建插件管理器。
func NewManager(dir string, db *gorm.DB) *Manager {
	m := &Manager{
		plugins: make(map[string]*Instance),
		dir:     dir,
		db:      db,
		host:    NewHostService(db),
		catalog: map[string]string{},
	}
	if db != nil {
		m.runLog = runlog.New(db, func() string { return setting.New(db).RunLevel() })
	}
	return m
}

// runLogger 运行日志写入器（db 为 nil 时安全返回空实现）。
func (m *Manager) runLogger() *runlog.Logger {
	if m.runLog == nil {
		return runlog.New(nil, nil)
	}
	return m.runLog
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

// Scan 扫描插件目录，返回可启动的二进制路径列表（目录不存在视为空）。
func (m *Manager) Scan() ([]string, error) {
	if _, err := os.Stat(m.dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugin dir: %w", err)
	}
	var found []string
	for _, dir := range m.pluginDirs() {
		if bin, err := pluginBinary(dir); err == nil {
			found = append(found, bin)
		}
	}
	return found, nil
}

// pluginDirs 全部插件目录（含 manifest.json 的目录），按目录名排序。
// 布局：<dir>/<name>/ 或 <dir>/<source>/<name>/（非官方源命名空间，只下探一层）。
func (m *Manager) pluginDirs() []string {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(m.dir, e.Name())
		if isPluginDir(sub) {
			dirs = append(dirs, sub)
			continue
		}
		inner, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, ie := range inner {
			if p := filepath.Join(sub, ie.Name()); ie.IsDir() && isPluginDir(p) {
				dirs = append(dirs, p)
			}
		}
	}
	return dirs
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
// go-plugin 层按 [MinProtocolVersion, ProtocolVersion] 协商版本，旧契约插件按协商到的版本握手（线格式向后兼容）。
func (m *Manager) Start(ctx context.Context, binPath string) (*Instance, error) {
	// 每个插件实例独立持有宿主服务（forPlugin 按插件名隔离 store 等状态）
	name := filepath.Base(filepath.Dir(binPath))
	set := goplugin.PluginSet{"claw_plugin": &ClawPluginPlugin{host: m.host.forPlugin(name)}}
	versioned := map[int]goplugin.PluginSet{}
	for v := sdk.MinProtocolVersion; v <= sdk.ProtocolVersion; v++ {
		versioned[int(v)] = set
	}
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  handshakeConfig,
		VersionedPlugins: versioned,
		Cmd:              execCommand(binPath),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("connect plugin %s: %w", binPath, err)
	}
	negotiated := int32(client.NegotiatedVersion())

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

	// 契约握手：按协商版本校验，manifest 声明须一致
	hs, err := pc.Handshake(ctx, &pb.HandshakeRequest{
		CoreVersion:     CoreVersion,
		ProtocolVersion: negotiated,
	})
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("handshake: %w", err)
	}
	if hs.Error != nil && hs.Error.Code != 0 {
		client.Kill()
		return nil, fmt.Errorf("plugin rejected: %s", hs.Error.Message)
	}
	if hs.Manifest == nil || hs.Manifest.ProtocolVersion != negotiated {
		client.Kill()
		return nil, fmt.Errorf("protocol version mismatch: negotiated=%d plugin=%d",
			negotiated, hs.Manifest.GetProtocolVersion())
	}

	inst := &Instance{Name: hs.Manifest.Name, Manifest: hs.Manifest, Protocol: negotiated, client: client, rpc: pc}
	m.mu.Lock()
	m.plugins[inst.Name] = inst
	m.mu.Unlock()
	m.syncRecord(inst)
	return inst, nil
}

// syncRecord 每次启动成功后同步 plugins 表（版本 / 契约 / manifest 快照）；
// 快照供插件停止时管理页照常渲染能力与授权方式。
func (m *Manager) syncRecord(inst *Instance) {
	if m.db == nil {
		return
	}
	mf := inst.Manifest
	manifestJSON, _ := protojson.Marshal(mf)
	var rec model.Plugin
	if err := m.db.Where("name = ?", mf.Name).First(&rec).Error; err != nil {
		m.db.Create(&model.Plugin{
			Name: mf.Name, Version: mf.Version, Author: mf.Author,
			ProtocolVersion: inst.Protocol, ManifestJSON: string(manifestJSON), Enabled: true,
		})
		return
	}
	m.db.Model(&rec).Updates(map[string]interface{}{
		"version": mf.Version, "author": mf.Author,
		"protocol_version": inst.Protocol, "manifest_json": string(manifestJSON),
	})
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
	dir, ok := m.pluginDir(name)
	if !ok {
		return nil, false
	}
	bin, err := pluginBinary(dir)
	if err != nil {
		return nil, false
	}
	fmt.Printf("[plugin] %s crashed, restarting\n", name)
	m.runLogger().Warn("plugin", "restart", "插件崩溃自动重启: "+name, "", nil)
	inst2, err := m.Start(context.Background(), bin)
	if err == nil {
		return inst2, true
	}
	fmt.Printf("[plugin] restart %s failed: %v\n", name, err)
	m.runLogger().Error("plugin", "restart", "插件重启失败: "+name, err.Error(), nil)
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

// AutoStarts 开机自动启动的二进制列表：Scan 结果排除持久化停止的插件（enabled=0）。
// DB 不可用 / 插件无记录（新装的）照常拉起。
func (m *Manager) AutoStarts() ([]string, error) {
	bins, err := m.Scan()
	if err != nil {
		return nil, err
	}
	if m.db == nil {
		return bins, nil
	}
	var names []string // Pluck 只能填充 slice，不能是 map
	if err := m.db.Model(&model.Plugin{}).Where("enabled = ?", false).Pluck("name", &names).Error; err != nil || len(names) == 0 {
		return bins, nil
	}
	disabled := make(map[string]bool, len(names))
	for _, n := range names {
		disabled[n] = true
	}
	out := bins[:0:0]
	for _, bin := range bins {
		if !disabled[filepath.Base(filepath.Dir(bin))] {
			out = append(out, bin)
		}
	}
	return out, nil
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
	injectFingerprint(req)

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

// injectFingerprint 按入口协议生成客户端指纹头放入 extra（messages=Claude Code / openai 系=Codex）。
// 只生成下发，是否采用由插件决定。
func injectFingerprint(req *pb.ChatRequest) {
	var h http.Header
	switch req.Source {
	case "messages":
		h = fingerprint.ClaudeHeaders("")
	case "chat_completions", "responses":
		h = fingerprint.CodexHeaders("")
	default:
		return
	}
	m := make(map[string]string, len(h))
	for k, v := range h {
		m[strings.ToLower(k)] = v[0]
	}
	if req.Extra == nil {
		req.Extra = m
		return
	}
	for k, v := range m {
		req.Extra[k] = v
	}
}

// Stop 停止一个插件。persists 为 true 时写持久化状态（enabled=0，重启核心保持停止；
// 用户手动停用）；false 仅停本次进程（升级/卸载前临时停，重启照常拉起）。
func (m *Manager) Stop(name string, persists bool) {
	m.mu.Lock()
	inst, ok := m.plugins[name]
	if ok {
		delete(m.plugins, name)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	inst.client.Kill()
	if persists && m.db != nil {
		m.db.Model(&model.Plugin{}).Where("name = ?", name).Update("enabled", false)
	}
}

// Resume 清除持久化停止状态（enabled=1，下次重启核心照常拉起）。
func (m *Manager) Resume(name string) {
	if m.db != nil {
		m.db.Model(&model.Plugin{}).Where("name = ?", name).Update("enabled", true)
	}
}

// ResolveModel 模型名 → 插件名（gateway.PluginRegistry 实现）。
func (m *Manager) ResolveModel(model string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	name, ok := m.catalog[model]
	return name, ok
}

// StopAll 停止全部插件（进程退出前调用；仅停进程，不写持久化状态）。
func (m *Manager) StopAll() {
	m.mu.Lock()
	names := make([]string, 0, len(m.plugins))
	for n := range m.plugins {
		names = append(names, n)
	}
	m.mu.Unlock()
	for _, n := range names {
		m.Stop(n, false)
	}
}
