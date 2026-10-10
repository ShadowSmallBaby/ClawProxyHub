package task

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"time"
)

const claimLease = 5 * time.Minute

// UseExternalScheduler 必须在 Start 前调用；Android 由系统唤醒执行，不启用常驻扫描。
func (e *Engine) UseExternalScheduler() { e.external = true }
func (e *Engine) TriggerDue(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e.ctx.Err() != nil {
		return e.ctx.Err()
	}
	e.tick(ctx)
	return nil
}
func (e *Engine) WaitIdle(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		e.mu.Lock()
		n := len(e.running)
		e.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (e *Engine) CancelRule(id int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cancel := e.active[id]
	if cancel == nil {
		return fmt.Errorf("rule is not executing in this core")
	}
	cancel()
	return nil
}
func (e *Engine) heartbeat(ctx context.Context, token string, cancel context.CancelFunc) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			res := e.db.Exec(`UPDATE task_claims SET lease_until=? WHERE token=? AND owner=? AND lease_until>?`, now.Add(claimLease).UnixMilli(), token, e.owner, now.UnixMilli())
			if res.Error != nil || res.RowsAffected != 1 {
				cancel()
				return
			}
		}
	}
}
func (e *Engine) hasClaim(token string) bool {
	var n int64
	return e.db.Raw(`SELECT count(*) FROM task_claims WHERE token=? AND owner=? AND lease_until>?`, token, e.owner, time.Now().UnixMilli()).Scan(&n).Error == nil && n == 1
}
func (e *Engine) finishClaim(token, reason string) error {
	return e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE task_runs SET status='failed',finished_at=?,error_message=? WHERE execution_token=? AND status IN ('queued','running')`, time.Now().UTC(), reason, token).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM task_claims WHERE token=? AND owner=?`, token, e.owner).Error
	})
}
func (e *Engine) releaseOwned() error {
	var tokens []string
	if err := e.db.Raw(`SELECT token FROM task_claims WHERE owner=?`, e.owner).Scan(&tokens).Error; err != nil {
		return err
	}
	for _, token := range tokens {
		if err := e.finishClaim(token, "core stopped before execution finished"); err != nil {
			return err
		}
	}
	return nil
}

// RecoverExpired 不重试可能产生外部副作用的任务；历史记录失败，下一正常周期独立领取。
func (e *Engine) RecoverExpired(now time.Time) error {
	return e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE task_runs SET status='failed',finished_at=?,error_message='execution lease expired; outcome unknown' WHERE status IN ('queued','running') AND (execution_token='' OR execution_token IN (SELECT token FROM task_claims WHERE lease_until<=?))`, now, now.UnixMilli()).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM task_claims WHERE lease_until<=?`, now.UnixMilli()).Error
	})
}
