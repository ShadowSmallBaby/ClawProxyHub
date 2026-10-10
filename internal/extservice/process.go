// Package extservice 管理受信 Go 扩展进程，所有回调绑定管道与宿主签发的调用上下文。
package extservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/extstore"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

type capability struct {
	ctx    context.Context
	grants []string
}
type Process struct {
	connection *Connection
	peer       *spec.Peer
	session    *extstore.Session
	done       chan struct{}
	lifetime   context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	calls      map[string]capability
	err        error
	stopping   bool
	committed  atomic.Bool
}

// Connection 只交付宿主创建的 RPC 连接，平台负责原生进程的最终终止。
type Connection struct {
	Reader io.ReadCloser
	Writer io.WriteCloser
	Wait   func() error
	Close  func() error
}
type tail struct {
	mu   sync.Mutex
	data []byte
}

func (t *tail) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, data...)
	if len(t.data) > 4096 {
		t.data = t.data[len(t.data)-4096:]
	}
	return len(data), nil
}
func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Start(ctx context.Context, path string, init spec.Initialize, background []string, session *extstore.Session, onExit func(*Process, error)) (*Process, error) {
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	for _, name := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "TZ"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	hideWindow(cmd)
	cmd.Stderr = &tail{}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}
	return StartConnection(ctx, &Connection{Reader: stdout, Writer: stdin, Wait: cmd.Wait, Close: func() error {
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}}, init, background, session, onExit)
}

// StartConnection 让 Android 私有服务复用初始化、权限、存储和失败回退。
func StartConnection(ctx context.Context, connection *Connection, init spec.Initialize, background []string, session *extstore.Session, onExit func(*Process, error)) (*Process, error) {
	if connection == nil || connection.Reader == nil || connection.Writer == nil || connection.Close == nil {
		if connection != nil && connection.Close != nil {
			_ = connection.Close()
		}
		return nil, fmt.Errorf("invalid extension connection")
	}
	lifetime, cancel := context.WithCancel(context.Background())
	p := &Process{connection: connection, session: session, done: make(chan struct{}), lifetime: lifetime, cancel: cancel, calls: map[string]capability{}}
	if len(background) > 0 {
		init.Background = token()
		p.calls[init.Background] = capability{ctx: lifetime, grants: append([]string{}, background...)}
	}
	p.peer = spec.NewPeer(connection.Reader, connection.Writer, p.callback)
	go func() {
		var err error
		if connection.Wait != nil {
			err = connection.Wait()
		} else {
			<-p.peer.Done()
			err = p.peer.Err()
		}
		p.mu.Lock()
		p.err = err
		stopping := p.stopping
		p.mu.Unlock()
		cancel()
		p.peer.Close()
		_ = connection.Close()
		if session != nil {
			session.Close()
		}
		close(p.done)
		if !stopping && onExit != nil {
			if err == nil {
				err = errors.New("extension backend exited")
			}
			onExit(p, err)
		}
	}()
	startup, stop := context.WithTimeout(ctx, 15*time.Second)
	defer stop()
	raw, err := p.peer.Call(startup, "initialize", init)
	if err == nil {
		var reply struct {
			Protocol int `json:"protocol"`
		}
		err = spec.DecodeStrict(raw, &reply)
		if err == nil && reply.Protocol != spec.BackendProtocol {
			err = fmt.Errorf("incompatible backend handshake")
		}
	}
	if err != nil {
		_ = p.Close(context.Background())
		return nil, fmt.Errorf("extension startup: %w", err)
	}
	return p, nil
}
func (p *Process) Commit() { p.committed.Store(true) }
func (p *Process) Alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}
func (p *Process) callback(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if method != "storage" || p.session == nil {
		return nil, fmt.Errorf("host capability unavailable")
	}
	var request spec.StorageCall
	if err := spec.DecodeStrict(raw, &request); err != nil {
		return nil, err
	}
	p.mu.Lock()
	call, ok := p.calls[request.Capability]
	stopping := p.stopping
	p.mu.Unlock()
	if !ok || stopping || call.ctx.Err() != nil || p.lifetime.Err() != nil {
		return nil, fmt.Errorf("host capability expired")
	}
	grants := call.grants
	if !p.committed.Load() {
		grants = []string{}
		if spec.HasPermission(call.grants, "storage.read") {
			grants = append(grants, "storage.read")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(call.ctx, cancel)
	defer stop()
	return p.session.Execute(ctx, request.Requests, grants)
}
func (p *Process) Invoke(ctx context.Context, action string, input json.RawMessage, grants []string) (json.RawMessage, error) {
	if !p.committed.Load() || p.lifetime.Err() != nil {
		return nil, fmt.Errorf("extension service unavailable")
	}
	key := token()
	p.mu.Lock()
	if p.stopping || len(p.calls) >= 32 {
		p.mu.Unlock()
		return nil, fmt.Errorf("extension service unavailable")
	}
	p.calls[key] = capability{ctx: ctx, grants: append([]string{}, grants...)}
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.calls, key); p.mu.Unlock() }()
	return p.peer.Call(ctx, "invoke", spec.Invocation{Action: action, Input: input, Capability: key})
}
func (p *Process) Close(ctx context.Context) error {
	p.mu.Lock()
	p.stopping = true
	p.calls = map[string]capability{}
	p.mu.Unlock()
	p.committed.Store(false)
	p.cancel()
	if p.session != nil {
		p.session.Close()
	}
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	_, _ = p.peer.Call(shutdown, "shutdown", struct{}{})
	cancel()
	select {
	case <-p.done:
		return nil
	case <-time.After(200 * time.Millisecond):
	}
	p.peer.Close()
	if err := p.connection.Close(); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("extension process did not terminate")
	}
}
