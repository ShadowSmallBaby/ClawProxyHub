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
func StartLogRetention(ctx context.Context, db *gorm.DB, settings *setting.Store) {
	go func() {
		sweepLogs(db, settings)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepLogs(db, settings)
			}
		}
	}()
}

// sweepLogs 删除 created_at 早于保留窗口的日志。
func sweepLogs(db *gorm.DB, settings *setting.Store) {
	days := settings.LogRetentionDays()
	if days <= 0 {
		return
	}
	res := db.Exec("DELETE FROM request_logs WHERE created_at < datetime('now', ?)", fmt.Sprintf("-%d days", days))
	if res.Error == nil && res.RowsAffected > 0 {
		fmt.Printf("[janitor] purged %d request logs older than %d days\n", res.RowsAffected, days)
		runlog.New(db, func() string { return settings.RunLevel() }).
			Info("janitor", "purge", fmt.Sprintf("清理过期调用日志 %d 条（%d 天前）", res.RowsAffected, days), "", nil)
	}
}
