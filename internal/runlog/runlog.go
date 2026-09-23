// Package runlog — 运行日志（系统级：级别过滤后落 run_logs 表；写库失败静默）。
package runlog

import (
	"strings"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// 级别严重度：数值越小越严重；配置为某级则记录该级及更严重（sev(entry) <= sev(config)）。
var severity = map[string]int{"error": 0, "warn": 1, "debug": 2, "info": 3}

// Logger 运行日志写入器。
type Logger struct {
	db    *gorm.DB
	level func() string // 读当前配置的记录级别（实时，设置改动立即生效）
}

// New 构造（level 为 nil 时按 error 级）。
func New(db *gorm.DB, level func() string) *Logger {
	if level == nil {
		level = func() string { return "error" }
	}
	return &Logger{db: db, level: level}
}

// Log 级别过滤后落库；level 非法按 error。
func (l *Logger) Log(level, module, action, message, detail string, accountID *int64) {
	if l == nil || l.db == nil {
		return
	}
	if _, ok := severity[level]; !ok {
		level = "error"
	}
	want, ok := severity[l.level()]
	if !ok {
		want = severity["error"]
	}
	if severity[level] > want {
		return
	}
	l.db.Create(&model.RunLog{
		Level: level, Module: strings.TrimSpace(module), Action: strings.TrimSpace(action),
		Message: message, Detail: detail, AccountID: accountID,
	})
}

// Error / Warn / Debug / Info 便捷方法。
func (l *Logger) Error(module, action, message, detail string, accountID *int64) {
	l.Log("error", module, action, message, detail, accountID)
}
func (l *Logger) Warn(module, action, message, detail string, accountID *int64) {
	l.Log("warn", module, action, message, detail, accountID)
}
func (l *Logger) Debug(module, action, message, detail string, accountID *int64) {
	l.Log("debug", module, action, message, detail, accountID)
}
func (l *Logger) Info(module, action, message, detail string, accountID *int64) {
	l.Log("info", module, action, message, detail, accountID)
}
