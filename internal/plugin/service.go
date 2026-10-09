package plugin

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
)

// ServiceBinding 是平台完成身份校验和协议协商后的连接，Release 解除服务绑定。
type ServiceBinding struct {
	Requests, Callbacks net.Conn
	Protocol            int32
	Release             func() error
}

// ServiceConnector 根据系统安装状态发现服务；Connect 失败必须自行释放部分建立的连接。
type ServiceConnector interface {
	Available(dir string) bool
	Connect(context.Context, string) (*ServiceBinding, error)
}

type serviceRuntime struct{ connector ServiceConnector }

func NewServiceRuntime(connector ServiceConnector) Runtime { return &serviceRuntime{connector} }

func (r *serviceRuntime) Available(dir string) bool {
	return r.connector != nil && r.connector.Available(dir)
}

func (r *serviceRuntime) Start(ctx context.Context, dir string, host pb.ClawHostServer) (Session, error) {
	if r.connector == nil {
		return nil, fmt.Errorf("missing Service connector")
	}
	binding, err := r.connector.Connect(ctx, dir)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, fmt.Errorf("missing Service binding")
	}
	s := &serviceSession{binding: binding}
	if binding.Requests == nil || binding.Callbacks == nil || binding.Release == nil ||
		binding.Protocol < sdk.MinProtocolVersion || binding.Protocol > sdk.ProtocolVersion {
		_ = s.Close()
		return nil, fmt.Errorf("invalid Service binding")
	}
	if err := ctx.Err(); err != nil {
		_ = s.Close()
		return nil, err
	}
	s.transport, err = transport.ConnectHost(binding.Callbacks, binding.Requests, host)
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	go func() {
		<-s.transport.Done()
		_ = s.Close()
	}()
	return s, nil
}

type serviceSession struct {
	binding   *ServiceBinding
	transport *transport.Session
	once      sync.Once
	closeErr  error
}

func (s *serviceSession) Client() pb.ClawPluginClient {
	return pb.NewClawPluginClient(s.transport.Client)
}
func (s *serviceSession) ProtocolVersion() int32 { return s.binding.Protocol }
func (s *serviceSession) Exited() bool {
	select {
	case <-s.transport.Done():
		return true
	default:
		return false
	}
}

func (s *serviceSession) Close() error {
	s.once.Do(func() {
		if s.transport != nil {
			s.transport.Close()
		} else {
			if s.binding.Requests != nil {
				_ = s.binding.Requests.Close()
			}
			if s.binding.Callbacks != nil {
				_ = s.binding.Callbacks.Close()
			}
		}
		if s.binding.Release != nil {
			s.closeErr = s.binding.Release()
		}
	})
	return s.closeErr
}
