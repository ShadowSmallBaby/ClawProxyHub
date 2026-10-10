// Package sdk — 插件开发工具包：实现 ClawPluginServer 即可接入核心。
package sdk

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

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
	// ExtraFingerprintHeaders ChatRequest.extra 键：按入口协议生成的客户端指纹头（JSON map，messages=Claude Code / openai 系=Codex）；插件按需采用。
	ExtraFingerprintHeaders string = "fingerprint_headers"
	// SettingBrowserUserAgent 宿主 GetSettings 合并视图的保留键：全局浏览器 UA（空 / 缺失 = 插件用内置值）。
	SettingBrowserUserAgent string = "_browser_user_agent"
	MagicCookieKey          string = "CPH_PLUGIN"
	MagicCookieVal          string = "claw-proxy-hub-plugin"
	// HostBrokerID 宿主 ClawHost 服务在 broker 上的固定通道号。
	HostBrokerID uint32 = 1000
)

// gRPC 消息上限（core↔插件）：默认 4MB 太小，长会话请求会撞 ResourceExhausted → 502。
// 由 env 启动期注入，核心与插件读同一变量保持两端一致（改后需重启进程；SDK 变更需重编插件）。
const (
	// EnvGRPCMaxMsgSize gRPC 收发消息字节上限的环境变量名（纯字节整数）。
	EnvGRPCMaxMsgSize = "CPH_GRPC_MAX_MSG_SIZE"
	// DefaultGRPCMaxMsgSize 缺省上限：64MB。
	DefaultGRPCMaxMsgSize = 64 << 20
	// minGRPCMaxMsgSize 下限保护：低于 gRPC 原生 4MB 默认值一律回退，避免配得更小。
	minGRPCMaxMsgSize = 4 << 20
)

// GRPCMaxMsgSize 解析 env 得到 gRPC 消息上限；空/非法/过小回退默认。
func GRPCMaxMsgSize() int {
	n, ok := parseByteSize(os.Getenv(EnvGRPCMaxMsgSize))
	if !ok || n < minGRPCMaxMsgSize {
		return DefaultGRPCMaxMsgSize
	}
	return n
}

// parseByteSize 解析字节大小，兼容纯字节整数与 kb/mb/gb 单位（大小写、是否带 b、是否带空格均不敏感，
// 如 67108864 / 64mb / 64MB / 64m / 4096kb / "64 MB"）；单位按 1024 进制。返回 (字节数, 是否有效)。
func parseByteSize(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	mult := 1
	switch { // 先匹配两字母单位再单字母，避免 "mb" 被 "b" 抢先命中
	case strings.HasSuffix(s, "gb"):
		mult, s = 1<<30, s[:len(s)-2]
	case strings.HasSuffix(s, "mb"):
		mult, s = 1<<20, s[:len(s)-2]
	case strings.HasSuffix(s, "kb"):
		mult, s = 1<<10, s[:len(s)-2]
	case strings.HasSuffix(s, "g"):
		mult, s = 1<<30, s[:len(s)-1]
	case strings.HasSuffix(s, "m"):
		mult, s = 1<<20, s[:len(s)-1]
	case strings.HasSuffix(s, "k"):
		mult, s = 1<<10, s[:len(s)-1]
	case strings.HasSuffix(s, "b"):
		s = s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return int(f * float64(mult)), true
}

// Host 宿主回调能力（由核心注入，插件实现里可取用）。
// 连接在注入时即异步建立（broker 连接信息仅短暂有效）；失败后下次调用会重试。
type Host struct {
	dial   func() (pb.ClawHostClient, error)
	mu     sync.Mutex
	client pb.ClawHostClient
}

// NewHost 注入平台提供的宿主客户端；连接关闭与重连由平台会话管理。
func NewHost(client pb.ClawHostClient) *Host {
	return &Host{client: client}
}

func (h *Host) conn() pb.ClawHostClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.client != nil {
		return h.client
	}
	if h.dial == nil {
		return nil // 未注入宿主连接（如单测直接构造 Host）：日志静默降级
	}
	c, err := h.dial()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[cph-sdk] host dial failed: %v\n", err)
		return nil
	}
	h.client = c
	return c
}

// Log 写统一日志管道（message 为精简消息；fields 可带 action/detail 等排查字段）。
func (h *Host) Log(level, message string) {
	h.LogFields(level, message, nil)
}

// LogFields 写统一日志管道（带结构化字段：action / detail）。
func (h *Host) LogFields(level, message string, fields map[string]string) {
	if c := h.conn(); c != nil {
		c.Log(context.Background(), &pb.LogEntry{Level: level, Message: message, Fields: fields})
	}
}

// StoreGet 读插件状态。
func (h *Host) StoreGet(key string) ([]byte, bool) {
	value, found, _ := h.StoreGetContext(context.Background(), key)
	return value, found
}

// StorePut 写插件状态。
func (h *Host) StorePut(key string, value []byte) {
	_ = h.StorePutContext(context.Background(), key, value)
}

// Settings 读插件设置（核心管理界面在线编辑；pluginName 为本插件 id）。
// 返回原始 JSON（结构由 manifest.settings_schema 定义），读取失败回 nil 由调用方用默认值。
func (h *Host) Settings(pluginName string) []byte {
	return h.InstanceSettings(pluginName, 0)
}

// InstanceSettings 读实例视图的设置：插件设置 ← 实例设置 ← {"base_url": 实例地址}。
// instanceID 为 0 时等价于 Settings（仅插件级）。
func (h *Host) InstanceSettings(pluginName string, instanceID int64) []byte {
	values, err := h.InstanceSettingsContext(context.Background(), pluginName, instanceID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[cph-sdk] GetSettings failed: %v\n", err)
		return nil
	}
	return values
}

// Plugin 插件作者需要实现的全部：gRPC 服务 + 宿主注入点。
type Plugin interface {
	pb.ClawPluginServer
}

// HostAware 可选：实现后核心会把宿主回调注入插件。
type HostAware interface {
	SetHost(host *Host)
}
