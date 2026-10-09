package transport

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testHost struct{ pb.UnimplementedClawHostServer }

func (*testHost) GetSettings(context.Context, *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error) {
	return &pb.GetSettingsResponse{Values: []byte("callback-ok")}, nil
}

type testPlugin struct {
	pb.UnimplementedClawPluginServer
	host      *sdk.Host
	started   chan struct{}
	cancelled chan struct{}
}

func (p *testPlugin) SetHost(host *sdk.Host) { p.host = host }
func (p *testPlugin) RunTask(ctx context.Context, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	if req.CapabilityId == "wait" {
		close(p.started)
		<-ctx.Done()
		close(p.cancelled)
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	return &pb.RunTaskResponse{Summary: string(p.host.Settings("test"))}, nil
}

func TestSessionCallbackCancellationAndDisconnect(t *testing.T) {
	a, b := net.Pipe()
	c, d := net.Pipe()
	host, err := ConnectHost(c, a, &testHost{})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	impl := &testPlugin{started: make(chan struct{}), cancelled: make(chan struct{})}
	plugin, err := ServePlugin(b, d, impl)
	if err != nil {
		t.Fatal(err)
	}
	defer plugin.Close()
	client := pb.NewClawPluginClient(host.Client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := client.RunTask(ctx, &pb.RunTaskRequest{})
	if err != nil || result.GetSummary() != "callback-ok" {
		t.Fatalf("callback: %v, %v", result, err)
	}
	waitCtx, stop := context.WithCancel(ctx)
	defer stop()
	finished := make(chan error, 1)
	go func() { _, err := client.RunTask(waitCtx, &pb.RunTaskRequest{CapabilityId: "wait"}); finished <- err }()
	select {
	case <-impl.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stop()
	select {
	case err := <-finished:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("cancel: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-impl.cancelled:
	case <-ctx.Done():
		t.Fatal("plugin did not receive cancellation")
	}
	plugin.Close()
	select {
	case <-host.Done():
	case <-ctx.Done():
		t.Fatal("host leaked after disconnect")
	}
}
