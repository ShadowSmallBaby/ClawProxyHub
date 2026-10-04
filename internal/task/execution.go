// execution.go — 单规则执行与账号凭据写回。
package task

import (
	"context"
	"encoding/json"
	"fmt"
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
	if err := e.db.Save(&run).Error; err != nil {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			run.Status, run.ErrorMessage = "failed", truncate(fmt.Sprint(p), 1000)
		}
		fin := time.Now().UTC()
		run.FinishedAt = &fin
		if err := e.db.Save(&run).Error; err != nil {
			fmt.Printf("[task] persist run %d: %v\n", run.ID, err)
			return
		}
		if run.Status == "success" && acct != nil && e.bus != nil {
			e.bus.Publish(event.Event{Topic: event.TopicTaskCompleted, AccountID: acct.ID})
		}
	}()
	resp, err := e.runAccount(ctx, rule, acct)
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
	req := &pb.RunTaskRequest{CapabilityId: rule.CapabilityID}
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
