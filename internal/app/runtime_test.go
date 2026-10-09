package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pluginpkg "github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/task"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
)

type appConnector struct{ released atomic.Int32 }

func (*appConnector) Available(string) bool { return true }
func (c *appConnector) Connect(context.Context, string) (*pluginpkg.ServiceBinding, error) {
	a, b := net.Pipe()
	d, e := net.Pipe()
	peer, err := transport.ServePlugin(b, e, &appPlugin{})
	if err != nil {
		a.Close()
		d.Close()
		return nil, err
	}
	return &pluginpkg.ServiceBinding{Requests: a, Callbacks: d, Protocol: sdk.ProtocolVersion, Release: func() error {
		c.released.Add(1)
		peer.Close()
		return nil
	}}, nil
}

type appPlugin struct {
	pb.UnimplementedClawPluginServer
	host *sdk.Host
	name string
}

func (p *appPlugin) SetHost(host *sdk.Host) { p.host = host }
func (p *appPlugin) Handshake(context.Context, *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	name := p.name
	if name == "" {
		name = "service-test"
	}
	return &pb.HandshakeResponse{Manifest: &pb.Manifest{Name: name, ProtocolVersion: sdk.ProtocolVersion}}, nil
}
func (*appPlugin) ListModels(context.Context, *pb.CredentialBlob) (*pb.ModelList, error) {
	return &pb.ModelList{}, nil
}
func (p *appPlugin) RunTask(context.Context, *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	p.host.StorePut("task-result", []byte("persisted"))
	data, _ := p.host.StoreGet("task-result")
	return &pb.RunTaskResponse{Summary: string(data)}, nil
}

// 真实 app 装配通过 Service 执行任务，宿主回调落入核心数据库。
func TestAppUsesServiceRuntime(t *testing.T) {
	cfg := testConfig(t)
	dir := filepath.Join(cfg.PluginDir, "service-test")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"service-test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	connector := &appConnector{}
	a, err := New(cfg, WithPluginRuntime(pluginpkg.NewServiceRuntime(connector)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	response, err := task.NewPluginRunner(a.plugins).RunTask(context.Background(), "service-test", &pb.RunTaskRequest{})
	if err != nil || response.GetSummary() != "persisted" {
		t.Fatalf("Service task: %v %v", response, err)
	}
	var stored model.PluginStore
	if err := a.db.Where("plugin = ? AND key = ?", "service-test", "task-result").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if string(stored.Value) != "persisted" {
		t.Fatal("callback did not persist under plugin namespace")
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if connector.released.Load() != 1 {
		t.Fatalf("Service bindings released %d times", connector.released.Load())
	}
}
