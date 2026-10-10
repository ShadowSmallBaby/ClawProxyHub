// execution.go — 单规则执行与账号凭据写回。
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/event"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func (e *Engine) executeAccount(ctx context.Context, rule *model.TaskRule, acct *model.Account, queued *model.TaskRun) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	run := *queued
	run.Status = "running"
	run.StartedAt = time.Now().UTC()
	if ctx.Err() != nil || !e.hasClaim(run.ExecutionToken) {
		return
	}
	if res := e.db.Model(&model.TaskRun{}).Where("id=? AND execution_token=? AND status='queued'", run.ID, run.ExecutionToken).Updates(map[string]any{"status": "running", "started_at": run.StartedAt}); res.Error != nil || res.RowsAffected != 1 {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			run.Status, run.ErrorMessage = "failed", truncate(fmt.Sprint(p), 1000)
		}
		fin := time.Now().UTC()
		run.FinishedAt = &fin
		result := e.db.Model(&model.TaskRun{}).Where("id=? AND execution_token=? AND status='running'", run.ID, run.ExecutionToken).Select("status", "summary", "detail_json", "error_message", "finished_at").Updates(&run)
		if result.Error != nil {
			fmt.Printf("[task] persist run %d: %v\n", run.ID, result.Error)
			return
		}
		if result.RowsAffected != 1 {
			return
		}
		if run.Status == "success" && acct != nil && e.bus != nil {
			e.bus.Publish(event.Event{Topic: event.TopicTaskCompleted, AccountID: acct.ID})
		}
	}()
	ctx = context.WithValue(ctx, executionKey{}, run.ExecutionToken+fmt.Sprintf("/%d", run.ID))
	resp, err := e.runAccount(ctx, rule, acct)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && !e.hasClaim(run.ExecutionToken) {
		err = fmt.Errorf("execution lease lost")
	}
	if err != nil {
		run.Status, run.ErrorMessage = "failed", truncate(err.Error(), 1000)
		return
	}
	if resp == nil {
		run.Status, run.ErrorMessage = "failed", "empty task response"
		return
	}
	if resp.Error != nil && resp.Error.Code != 0 {
		run.Status, run.ErrorMessage = "failed", truncate(resp.Error.Message, 1000)
	} else {
		run.Status, run.Summary = "success", truncate(resp.Summary, 1000)
		if len(resp.DetailJson) <= 1<<20 && json.Valid([]byte(resp.DetailJson)) {
			run.DetailJSON = resp.DetailJson
		}
		if resp.DetailJson != "" && run.DetailJSON == "" {
			fmt.Printf("[task] omitted invalid or oversized detail for rule %d\n", rule.ID)
		}
	}
	if n := resp.Notification; n != nil && n.Title != "" {
		e.db.Create(&model.Notification{Title: truncate(n.Title, 256), Content: truncate(n.Content, 4000), Level: orDefault(n.Level, "info"), AccountID: run.AccountID})
	}
}

func (e *Engine) runAccount(ctx context.Context, rule *model.TaskRule, acct *model.Account) (*pb.RunTaskResponse, error) {
	token, _ := ctx.Value(executionKey{}).(string)
	req := &pb.RunTaskRequest{CapabilityId: rule.CapabilityID, Context: map[string]string{"execution_token": token}}
	if acct != nil {
		unlock, err := account.LockCredential(ctx, e.dataDir, acct.ID)
		if err != nil {
			return nil, err
		}
		defer unlock()
		if err := e.db.First(acct, acct.ID).Error; err != nil {
			return nil, err
		}
		cred, err := account.BuildCred(e.db, e.dataDir, acct, 0)
		if err != nil {
			return nil, err
		}
		req.Credential = cred
	}
	resp, err := e.runner.RunTask(ctx, pluginNameByID(e.db, rule.PluginID), req)
	if err == nil {
		err = ctx.Err()
	}
	if claim, _, _ := strings.Cut(token, "/"); err == nil && claim != "" && !e.hasClaim(claim) {
		err = fmt.Errorf("execution lease lost")
	}
	if err != nil || resp == nil {
		return resp, err
	}
	if (resp.Error == nil || resp.Error.Code == 0) && resp.Changed && acct != nil && len(resp.Blob) > 0 {
		blob, err := account.EncryptCredential(e.dataDir, resp.Blob)
		if err != nil {
			return nil, err
		}
		if err := e.db.Model(&model.Account{}).Where("id = ?", acct.ID).Updates(map[string]interface{}{"credential_blob": blob, "last_refresh_at": time.Now().UTC()}).Error; err != nil {
			return nil, err
		}
	}
	return resp, nil
}

type executionKey struct{}
