// Package janitor — 后台清理：按设置的保留天数定期删除过期调用日志（0 = 永久不清理）。
package janitor

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/runlog"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
)

// StartLogRetention 启动即清一次，之后每小时检查一次；ctx 取消退出。
func StartLogRetention(ctx context.Context, db *gorm.DB, settings *setting.Store) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweepLogs(db.WithContext(ctx), settings)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepLogs(db.WithContext(ctx), settings)
			}
		}
	}()
	return done
}

// sweepLogs 删除 created_at 早于保留窗口的日志。
func sweepLogs(db *gorm.DB, settings *setting.Store) {
	for _, entry := range []struct {
		table, column string
		days          int
	}{
		{"request_logs", "created_at", settings.LogRetentionDays()},
		{"run_logs", "created_at", settings.RetentionDays("logs.run_retention_days")},
		{"task_runs", "finished_at", settings.RetentionDays("logs.task_retention_days")},
		{"action_audits", "started_at", settings.RetentionDays("logs.run_retention_days")},
	} {
		if entry.days <= 0 {
			continue
		}
		query := fmt.Sprintf("DELETE FROM %s WHERE id IN (SELECT id FROM %s WHERE %s < ? LIMIT 1000)", entry.table, entry.table, entry.column)
		cutoff := time.Now().AddDate(0, 0, -entry.days)
		for batch := 0; batch < 100; batch++ {
			res := db.Exec(query, cutoff)
			if res.Error != nil {
				runlog.New(db, settings.RunLevel).Error("janitor", "purge", "日志清理失败", res.Error.Error(), nil)
				break
			}
			if res.RowsAffected < 1000 {
				break
			}
		}
	}
}
