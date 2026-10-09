package plugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
)

type pipeConnector struct {
	name     string
	protocol int32
	started  atomic.Int32
	released atomic.Int32
	mu       sync.Mutex
	peers    []*transport.Session
}

func (*pipeConnector) Available(string) bool { return true }
func (c *pipeConnector) Connect(ctx context.Context, dir string) (*ServiceBinding, error) {
	a, b := net.Pipe()
	d, e := net.Pipe()
	peer, err := transport.ServePlugin(b, e, &sessionPlugin{name: c.name, protocol: c.protocol})
	if err != nil {
		a.Close()
		d.Close()
		return nil, err
	}
	c.started.Add(1)
	c.mu.Lock()
	c.peers = append(c.peers, peer)
	c.mu.Unlock()
	return &ServiceBinding{Requests: a, Callbacks: d, Protocol: sdk.ProtocolVersion, Release: func() error {
		c.released.Add(1)
		peer.Close()
		return nil
	}}, nil
}

type sessionPlugin struct {
	pb.UnimplementedClawPluginServer
	name     string
	protocol int32
}

func (p *sessionPlugin) Handshake(context.Context, *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	return &pb.HandshakeResponse{Manifest: &pb.Manifest{Name: p.name, ProtocolVersion: p.protocol}}, nil
}
func (*sessionPlugin) RunTask(ctx context.Context, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	if req.CapabilityId == "wait" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &pb.RunTaskResponse{Summary: "service-task"}, nil
}

func pluginTestDir(t *testing.T, name string) (string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(fmt.Sprintf(`{"name":%q,"author":"disk-author"}`, name)), 0644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func TestServiceRuntimeLifecycle(t *testing.T) {
	root, dir := pluginTestDir(t, "test")
	c := &pipeConnector{name: "test", protocol: sdk.ProtocolVersion}
	m := NewManager(root, nil, WithRuntime(NewServiceRuntime(c)))
	t.Cleanup(m.StopAll)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dirs, err := m.Scan()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("Service discovery without executable: %v %v", dirs, err)
	}
	inst, err := m.Start(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Manifest.Author != "disk-author" {
		t.Fatal("author should use installed metadata")
	}
	result, err := inst.Client().RunTask(ctx, &pb.RunTaskRequest{})
	if err != nil || result.GetSummary() != "service-task" {
		t.Fatalf("task: %v %v", result, err)
	}
	again, err := m.Start(ctx, dir)
	if err != nil || again != inst || c.started.Load() != 1 {
		t.Fatal("duplicate Start opened another session")
	}
	c.peers[0].Close()
	select {
	case <-inst.session.(*serviceSession).transport.Done():
	case <-ctx.Done():
		t.Fatal("disconnect not observed")
	}
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			if _, ok := m.Get("test"); !ok {
				t.Error("failed to reconnect")
			}
		})
	}
	group.Wait()
	if c.started.Load() != 2 {
		t.Fatalf("concurrent Get started %d sessions", c.started.Load())
	}
	m.StopAll()
	m.StopAll()
	if c.released.Load() != 2 {
		t.Fatalf("bindings released %d times", c.released.Load())
	}
	if _, ok := m.Get("test"); ok {
		t.Fatal("stopped plugin must not restart")
	}
}

func TestServiceRuntimeRejectsHandshakeAndReleasesBinding(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int32
	}{{"wrong-name", sdk.ProtocolVersion}, {"test", 1}} {
		t.Run(fmt.Sprintf("%s-%d", tc.name, tc.version), func(t *testing.T) {
			root, dir := pluginTestDir(t, "test")
			c := &pipeConnector{name: tc.name, protocol: tc.version}
			m := NewManager(root, nil, WithRuntime(NewServiceRuntime(c)))
			if _, err := m.Start(context.Background(), dir); err == nil {
				t.Fatal("invalid handshake accepted")
			}
			if c.released.Load() != 1 || len(m.Names()) != 0 {
				t.Fatal("failed startup retained binding")
			}
		})
	}
}

type callbackHost struct{ pb.UnimplementedClawHostServer }

func (*callbackHost) GetSettings(context.Context, *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error) {
	return &pb.GetSettingsResponse{Values: []byte("callback-ok")}, nil
}

// 宿主 RPC 不依赖 Manager 的具体数据库对象。
func TestServiceRuntimeCallbackAndCancellation(t *testing.T) {
	a, b := net.Pipe()
	d, e := net.Pipe()
	p := &callbackPlugin{}
	peer, err := transport.ServePlugin(b, e, p)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	r := NewServiceRuntime(&fixedConnector{binding: &ServiceBinding{Requests: a, Callbacks: d, Protocol: sdk.ProtocolVersion, Release: func() error { return nil }}})
	s, err := r.Start(context.Background(), "test", &callbackHost{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := s.Client().RunTask(ctx, &pb.RunTaskRequest{})
	if err != nil || response.GetSummary() != "callback-ok" {
		t.Fatalf("callback: %v %v", response, err)
	}
	cancel()
	if _, err := s.Client().RunTask(ctx, &pb.RunTaskRequest{}); err == nil {
		t.Fatal("cancelled call succeeded")
	}
}

type fixedConnector struct{ binding *ServiceBinding }

func (*fixedConnector) Available(string) bool { return true }
func (c *fixedConnector) Connect(context.Context, string) (*ServiceBinding, error) {
	return c.binding, nil
}

type callbackPlugin struct {
	pb.UnimplementedClawPluginServer
	host *sdk.Host
}

func (p *callbackPlugin) SetHost(host *sdk.Host) { p.host = host }
func (p *callbackPlugin) RunTask(context.Context, *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	return &pb.RunTaskResponse{Summary: string(p.host.Settings("test"))}, nil
}
