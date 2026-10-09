package task

import (
	"context"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"strings"
	"sync"
	"testing"
	"time"
)

func claimRule(t *testing.T, e *Engine) model.TaskRule {
	t.Helper()
	p := model.Plugin{Name: "claim-test", ManifestJSON: "{}"}
	if err := e.db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(-time.Minute)
	rule := model.TaskRule{PluginID: p.ID, CapabilityID: "wait", Enabled: true, TriggerType: "interval", TriggerValue: "1h", TargetScope: "global", NextRunAt: &due}
	if err := e.db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	return rule
}
func TestConcurrentEnginesClaimOnceAndRejectStaleSchedule(t *testing.T) {
	e := timezoneEngine(t)
	other := NewEngine(e.db, e.dataDir, nil, nil, e.settings)
	defer other.Stop()
	e.workers.Do(func() {})
	other.workers.Do(func() {})
	rule := claimRule(t, e)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, engine := range []*Engine{e, other} {
		wg.Add(1)
		go func(engine *Engine) { defer wg.Done(); _, err := engine.enqueue(&rule, true); results <- err }(engine)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("accepted %d claims", success)
	}
	var runs []model.TaskRun
	e.db.Find(&runs)
	if len(runs) != 1 || runs[0].ExecutionToken == "" {
		t.Fatal(runs)
	}
	_ = e.finishClaim(runs[0].ExecutionToken, "test complete")
	_ = other.finishClaim(runs[0].ExecutionToken, "test complete")
	fresh := NewEngine(e.db, e.dataDir, nil, nil, e.settings)
	defer fresh.Stop()
	if _, err := fresh.enqueue(&rule, true); err == nil {
		t.Fatal("stale scheduled snapshot executed twice")
	}
}
func TestRecoveryLeavesLiveLeaseAndDoesNotReplayExpiredWork(t *testing.T) {
	e := timezoneEngine(t)
	runner := &cancellationRunner{started: make(chan struct{})}
	e.runner = runner
	e.UseExternalScheduler()
	e.Start(t.Context())
	rule := claimRule(t, e)
	id, err := e.RunNow(t.Context(), &rule)
	if err != nil {
		t.Fatal(err)
	}
	<-runner.started
	other := NewEngine(e.db, e.dataDir, nil, nil, e.settings)
	defer other.Stop()
	other.UseExternalScheduler()
	other.Start(t.Context())
	var run model.TaskRun
	e.db.First(&run, id)
	if run.Status != "running" {
		t.Fatalf("second engine invalidated live run: %+v", run)
	}
	if err = other.RecoverExpired(time.Now().Add(claimLease + time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = e.CancelRule(rule.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err = e.WaitIdle(ctx); err != nil {
		t.Fatal(err)
	}
	e.db.First(&run, id)
	if run.Status != "failed" || !strings.Contains(run.ErrorMessage, "outcome unknown") {
		t.Fatalf("late callback replaced recovery: %+v", run)
	}
	var count int64
	e.db.Model(&model.TaskRun{}).Count(&count)
	if count != 1 {
		t.Fatal("replayed expired task")
	}
	if err = e.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSystemJobCancellationReachesPlugin(t *testing.T) {
	e := timezoneEngine(t)
	runner := &cancellationRunner{started: make(chan struct{})}
	e.runner = runner
	e.UseExternalScheduler()
	e.Start(t.Context())
	claimRule(t, e)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := e.TriggerDue(ctx); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	cancel()
	wait, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if err := e.WaitIdle(wait); err != nil {
		t.Fatal(err)
	}
	var run model.TaskRun
	if err := e.db.First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || run.FinishedAt == nil {
		t.Fatalf("cancellation not persisted: %+v", run)
	}
}
