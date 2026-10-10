package task

import (
	"context"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

type cancellationRunner struct{ started chan struct{} }

func (*cancellationRunner) ListCapabilities(context.Context, string, int64) ([]*pb.TaskCapability, error) {
	return nil, nil
}
func (r *cancellationRunner) RunTask(ctx context.Context, _ string, _ *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	close(r.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestShutdownWaitsForRunPersistence(t *testing.T) {
	e := timezoneEngine(t)
	runner := &cancellationRunner{started: make(chan struct{})}
	e.runner = runner
	plugin := model.Plugin{Name: "shutdown", ManifestJSON: "{}"}
	if err := e.db.Create(&plugin).Error; err != nil {
		t.Fatal(err)
	}
	rule := model.TaskRule{PluginID: plugin.ID, CapabilityID: "wait", TriggerType: "interval", TriggerValue: "1h", TargetScope: "global"}
	if err := e.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	e.Start(context.Background())
	id, err := e.RunNow(context.Background(), &rule)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := e.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	var run model.TaskRun
	if err := e.db.First(&run, id).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || run.FinishedAt == nil {
		t.Fatalf("shutdown returned before final write: %+v", run)
	}
	if _, err := e.RunNow(context.Background(), &rule); err == nil {
		t.Fatal("accepted task after shutdown")
	}
	if err := e.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
