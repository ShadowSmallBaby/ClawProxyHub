package plugin

import (
	"context"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// Runtime 只负责平台启动与连接；名称、协议和业务握手由 Manager 统一验证。
type Runtime interface {
	Available(dir string) bool
	Start(ctx context.Context, dir string, host pb.ClawHostServer) (Session, error)
}

// Session 隔离进程/Service 生命周期，业务调用始终复用相同 protobuf 客户端。
type Session interface {
	Client() pb.ClawPluginClient
	ProtocolVersion() int32
	Exited() bool
	Close() error
}

type Option func(*Manager)

// WithRuntime 必须在首次启动前注入，不在运行中替换适配器。
func WithRuntime(runtime Runtime) Option { return func(m *Manager) { m.runtime = runtime } }

func (m *Manager) runtimeAdapter() Runtime {
	if m.runtime != nil {
		return m.runtime
	}
	return defaultRuntime(m)
}
