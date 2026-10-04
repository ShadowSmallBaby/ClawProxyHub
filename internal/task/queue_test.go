package task

import (
	"context"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// 无可用账号的到期规则不能一直占住扫描窗口，挡住后面的全局任务。
func TestTickPassesRulesWithoutEligibleAccounts(t *testing.T) {
	e := timezoneEngine(t)
	e.workers.Do(func() {}) // 只检查入队，不启动 RPC worker。
	plugin := model.Plugin{Name: "queue-test", ManifestJSON: "{}"}
	if err := e.db.Create(&plugin).Error; err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < 20; i++ {
		rule := model.TaskRule{PluginID: plugin.ID, CapabilityID: "check", Enabled: true,
			TriggerType: "interval", TriggerValue: "1h", TargetScope: "all", NextRunAt: &due}
		if err := e.db.Create(&rule).Error; err != nil {
			t.Fatal(err)
		}
	}
	ready := model.TaskRule{PluginID: plugin.ID, CapabilityID: "check", Enabled: true,
		TriggerType: "interval", TriggerValue: "1h", TargetScope: "global", NextRunAt: &due}
	if err := e.db.Create(&ready).Error; err != nil {
		t.Fatal(err)
	}
	e.doTick(context.Background())
	select {
	case job := <-e.queue:
		if job.rule.ID != ready.ID || len(job.runs) != 1 {
			t.Fatalf("unexpected queued job: %+v", job)
		}
	default:
		t.Fatal("eligible rule was blocked by rules without accounts")
	}
}
