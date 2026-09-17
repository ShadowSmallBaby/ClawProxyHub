// runner.go — Runner 适配器：把 plugin.Manager 桥接成 task.Runner。
package task

import (
	"context"
	"fmt"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// PluginRunner 经插件管理器触发任务能力。
type PluginRunner struct {
	mgr *plugin.Manager
}

func NewPluginRunner(mgr *plugin.Manager) *PluginRunner {
	return &PluginRunner{mgr: mgr}
}

func (r *PluginRunner) ListCapabilities(ctx context.Context, pluginName string) ([]*pb.TaskCapability, error) {
	inst, ok := r.mgr.Get(pluginName)
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}
	resp, err := inst.Client().ListTaskCapabilities(ctx, &pb.Empty{})
	if err != nil {
		return nil, err
	}
	return resp.Capabilities, nil
}

func (r *PluginRunner) RunTask(ctx context.Context, pluginName string, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error) {
	inst, ok := r.mgr.Get(pluginName)
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}
	return inst.Client().RunTask(ctx, req)
}
