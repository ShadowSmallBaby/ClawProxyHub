// service.go — Go 扩展入口与宿主回调；调用身份通过宿主签发的临时能力绑定。
package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type Initialize struct {
	Protocol    int            `json:"protocol"`
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	PackageHash string         `json:"package_hash"`
	Actions     []Action       `json:"actions"`
	Background  string         `json:"background,omitempty"`
	Settings    map[string]any `json:"settings,omitempty"`
}
type Invocation struct {
	Action     string          `json:"action"`
	Input      json.RawMessage `json:"input"`
	Capability string          `json:"capability"`
}
type StorageCall struct {
	Capability string           `json:"capability"`
	Requests   []StorageRequest `json:"requests"`
}
type ServiceHandler func(context.Context, *Host, json.RawMessage) (any, error)
type Service struct {
	Actions map[string]ServiceHandler
	Start   func(context.Context, *Host) error
	Stop    func(context.Context) error
}
type capabilityKey struct{}
type Host struct {
	peer    *Peer
	Initial Initialize
}

func (h *Host) Storage(ctx context.Context, request StorageRequest) (StorageResult, error) {
	results, err := h.Batch(ctx, []StorageRequest{request})
	if err != nil {
		return StorageResult{}, err
	}
	if len(results) != 1 {
		return StorageResult{}, fmt.Errorf("invalid storage reply")
	}
	return results[0], nil
}
func (h *Host) Batch(ctx context.Context, requests []StorageRequest) ([]StorageResult, error) {
	capability, _ := ctx.Value(capabilityKey{}).(string)
	if capability == "" {
		return nil, fmt.Errorf("storage requires an invocation or declared background context")
	}
	raw, err := h.peer.Call(ctx, "storage", StorageCall{Capability: capability, Requests: requests})
	if err != nil {
		return nil, err
	}
	var results []StorageResult
	if err = DecodeStrict(raw, &results); err != nil {
		return nil, err
	}
	return results, nil
}
func (h *Host) BackgroundContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, capabilityKey{}, h.Initial.Background)
}

// Serve 只将协议写入 stdout；扩展诊断应写入 stderr。
func Serve(service Service) error {
	return ServeIO(context.Background(), os.Stdin, os.Stdout, service)
}

// ServeIO 接管宿主提供的双向连接，桌面管道与 Android socket 共用动作和存储协议。
func ServeIO(ctx context.Context, reader io.ReadCloser, writer io.WriteCloser, service Service) error {
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	host := &Host{}
	setup := make(chan struct{})
	stopped := make(chan struct{})
	var stopOnce sync.Once
	var mu sync.Mutex
	initialized := false
	ready := false
	peer := NewPeer(reader, writer, func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		<-setup
		switch method {
		case "initialize":
			mu.Lock()
			if initialized {
				mu.Unlock()
				return nil, fmt.Errorf("already initialized")
			}
			initialized = true
			mu.Unlock()
			var init Initialize
			if err := DecodeStrict(raw, &init); err != nil {
				return nil, err
			}
			if init.Protocol != BackendProtocol || !ValidID(init.ID) || !ValidHash(init.PackageHash) {
				return nil, fmt.Errorf("incompatible host protocol")
			}
			declared := map[string]bool{}
			for _, action := range init.Actions {
				if action.Target != "" {
					continue
				}
				if service.Actions[action.ID] == nil {
					return nil, fmt.Errorf("missing action handler %s", action.ID)
				}
				declared[action.ID] = true
			}
			for id := range service.Actions {
				if !declared[id] {
					return nil, fmt.Errorf("undeclared action handler %s", id)
				}
			}
			host.Initial = init
			if service.Start != nil {
				// 启动超时终止后台任务，成功握手后后台任务随服务存活。
				stop := context.AfterFunc(ctx, cancel)
				err := service.Start(host.BackgroundContext(lifetime), host)
				stop()
				if err != nil {
					return nil, err
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			mu.Lock()
			ready = true
			mu.Unlock()
			return map[string]any{"protocol": BackendProtocol}, nil
		case "invoke":
			mu.Lock()
			active := ready
			mu.Unlock()
			if !active {
				return nil, fmt.Errorf("service not ready")
			}
			var in Invocation
			if err := DecodeStrict(raw, &in); err != nil {
				return nil, err
			}
			handler := service.Actions[in.Action]
			if handler == nil || in.Capability == "" {
				return nil, fmt.Errorf("unknown action or capability")
			}
			return handler(context.WithValue(ctx, capabilityKey{}, in.Capability), host, in.Input)
		case "shutdown":
			cancel()
			mu.Lock()
			ready = false
			mu.Unlock()
			var err error
			if service.Stop != nil {
				err = service.Stop(ctx)
			}
			stopOnce.Do(func() { close(stopped) })
			return map[string]bool{"stopped": err == nil}, err
		default:
			return nil, fmt.Errorf("unknown host method")
		}
	})
	defer peer.Close()
	stopContext := context.AfterFunc(ctx, func() { _ = peer.Close() })
	defer stopContext()
	host.peer = peer
	close(setup)
	select {
	case <-peer.Done():
		return peer.Err()
	case <-stopped:
		return nil
	}
}
