// Package transport 在已验证身份的双向连接上复用插件协议，不依赖子进程 broker。
package transport

import (
	"context"
	"net"
	"sync"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Session 接管两条连接；任一服务端连接断开后关闭整个会话，重连必须重新验证身份并创建连接。
type Session struct {
	Client   *grpc.ClientConn
	server   *grpc.Server
	incoming *singleListener
	outgoing net.Conn
	once     sync.Once
	done     chan struct{}
}

func (s *Session) Done() <-chan struct{} { return s.done }

func (s *Session) Close() {
	s.once.Do(func() {
		close(s.done)
		s.Client.Close()
		s.server.Stop()
		s.incoming.Close()
		s.outgoing.Close()
	})
}

// ConnectHost 启动宿主回调服务，并提供插件客户端所在连接。
func ConnectHost(callbacks, requests net.Conn, host pb.ClawHostServer) (*Session, error) {
	return connect(callbacks, requests, func(server *grpc.Server, _ *grpc.ClientConn) {
		pb.RegisterClawHostServer(server, host)
	})
}

// ServePlugin 注入宿主回调并启动业务协议；身份校验由传入连接的平台完成。
func ServePlugin(requests, callbacks net.Conn, impl sdk.Plugin) (*Session, error) {
	return connect(requests, callbacks, func(server *grpc.Server, client *grpc.ClientConn) {
		if aware, ok := impl.(sdk.HostAware); ok {
			aware.SetHost(sdk.NewHost(pb.NewClawHostClient(client)))
		}
		pb.RegisterClawPluginServer(server, impl)
	})
}

func connect(incoming, outgoing net.Conn, register func(*grpc.Server, *grpc.ClientConn)) (*Session, error) {
	var dialOnce sync.Once
	max := sdk.GRPCMaxMsgSize()
	client, err := grpc.NewClient("passthrough:///platform-session",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(max), grpc.MaxCallSendMsgSize(max)),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			var conn net.Conn
			dialOnce.Do(func() { conn = outgoing })
			if conn == nil {
				return nil, net.ErrClosed
			}
			return conn, nil
		}))
	if err != nil {
		incoming.Close()
		outgoing.Close()
		return nil, err
	}
	l := &singleListener{conn: incoming, done: make(chan struct{})}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(max), grpc.MaxSendMsgSize(max))
	s := &Session{Client: client, server: server, incoming: l, outgoing: outgoing, done: make(chan struct{})}
	register(server, client)
	go func() {
		server.Serve(l)
		s.Close()
	}()
	return s, nil
}

// singleListener 将已连接 socket 交给 gRPC，传输关闭时终止 Accept 和反向连接。
type singleListener struct {
	conn     net.Conn
	accepted bool
	mu       sync.Mutex
	once     sync.Once
	done     chan struct{}
}

func (l *singleListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	first := !l.accepted
	l.accepted = true
	l.mu.Unlock()
	if first {
		select {
		case <-l.done:
			return nil, net.ErrClosed
		default:
			return &closingConn{Conn: l.conn, closeListener: l.Close}, nil
		}
	}
	<-l.done
	return nil, net.ErrClosed
}

func (l *singleListener) Close() error {
	l.once.Do(func() { close(l.done); l.conn.Close() })
	return nil
}

func (l *singleListener) Addr() net.Addr { return l.conn.LocalAddr() }

type closingConn struct {
	net.Conn
	closeListener func() error
}

func (c *closingConn) Close() error { return c.closeListener() }
