package extservice

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func socketService(service spec.Service) (*Connection, context.CancelFunc, <-chan struct{}) {
	server, client := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	closed := make(chan struct{})
	var once sync.Once
	go func() { _ = spec.ServeIO(ctx, server, server, service) }()
	return &Connection{Reader: client, Writer: client, Close: func() error {
		once.Do(func() { cancel(); _ = client.Close(); close(closed) })
		return nil
	}}, cancel, closed
}

func TestSocketBackendLifecycleAndCancellation(t *testing.T) {
	waiting, cancelled := make(chan struct{}), make(chan struct{})
	service := spec.Service{Actions: map[string]spec.ServiceHandler{
		"echo": func(_ context.Context, _ *spec.Host, raw json.RawMessage) (any, error) { return raw, nil },
		"wait": func(ctx context.Context, _ *spec.Host, _ json.RawMessage) (any, error) {
			close(waiting)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		},
	}}
	connection, stop, closed := socketService(service)
	defer stop()
	init := spec.Initialize{Protocol: 1, ID: "socket-test", PackageHash: strings.Repeat("a", 64), Actions: []spec.Action{{ID: "echo"}, {ID: "wait"}}}
	process, err := StartConnection(context.Background(), connection, init, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close(context.Background())
	if _, err = process.Invoke(context.Background(), "echo", json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("invocation before commit succeeded")
	}
	process.Commit()
	result, err := process.Invoke(context.Background(), "echo", json.RawMessage(`{"value":"socket"}`), nil)
	if err != nil || string(result) != `{"value":"socket"}` {
		t.Fatalf("socket invocation: %s, %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { _, err := process.Invoke(ctx, "wait", json.RawMessage(`{}`), nil); finished <- err }()
	select {
	case <-waiting:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	if err = <-finished; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not reach backend")
	}
	if err = process.Close(context.Background()); err != nil || process.Alive() {
		t.Fatal("socket backend remains active:", err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("native worker was not released")
	}
}

func TestSocketBackendReleasesFailedHandshakeAndUnexpectedExit(t *testing.T) {
	init := spec.Initialize{Protocol: 1, ID: "socket-test", PackageHash: strings.Repeat("a", 64), Actions: []spec.Action{{ID: "missing"}}}
	connection, stop, closed := socketService(spec.Service{})
	defer stop()
	if _, err := StartConnection(context.Background(), connection, init, nil, nil, nil); err == nil {
		t.Fatal("invalid handshake succeeded")
	}
	select {
	case <-closed:
	default:
		t.Fatal("failed worker was not released")
	}
	init.Actions = nil
	connection, stop, closed = socketService(spec.Service{})
	defer stop()
	exited := make(chan error, 1)
	process, err := StartConnection(context.Background(), connection, init, nil, nil, func(_ *Process, err error) { exited <- err })
	if err != nil {
		t.Fatal(err)
	}
	process.Commit()
	stop()
	select {
	case err = <-exited:
		if err == nil {
			t.Fatal("exit cause missing")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exit did not reach host")
	}
	if process.Alive() {
		t.Fatal("dead socket remains available")
	}
	select {
	case <-closed:
	default:
		t.Fatal("disconnected worker was not released")
	}
}
