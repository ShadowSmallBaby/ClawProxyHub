// Package task — 调度引擎：核心只管"何时触发"，能力语义在插件。
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/event"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/runlog"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// Runner 是调度引擎对插件调用层的抽象（由 plugin.Manager 适配注入）。
type Runner interface {
	// ListCapabilities 返回插件声明的任务能力；instanceID>0 时插件可按实例配置裁剪。
	ListCapabilities(ctx context.Context, pluginName string, instanceID int64) ([]*pb.TaskCapability, error)
	// RunTask 触发一次能力执行。credential 为 nil 表示不针对具体账号。
	RunTask(ctx context.Context, pluginName string, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error)
}

// Engine 周期扫描 task_rules，到期即触发。
type Engine struct {
	db      *gorm.DB
	dataDir string
	runner  Runner
	bus     *event.Bus
	stop    chan struct{}
}

// NewEngine 创建调度引擎。
func NewEngine(db *gorm.DB, dataDir string, runner Runner, bus *event.Bus) *Engine {
	return &Engine{db: db, dataDir: dataDir, runner: runner, bus: bus, stop: make(chan struct{})}
}

// Start 启动扫描循环。残留的 running 记录（上次进程异常退出）标记为 failed。
func (e *Engine) Start(ctx context.Context) {
	e.db.Model(&model.TaskRun{}).Where("status = ?", "running").
		Update("status", "failed")

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.stop:
				return
			case <-ticker.C:
				e.tick(ctx)
			}
		}
	}()
}

// tick 执行一轮。单轮 panic 不退出进程，下一轮继续。
func (e *Engine) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[task] tick panicked: %v\n", r)
			runlog.New(e.db, func() string { return setting.New(e.db).RunLevel() }).
				Error("task", "tick", "任务扫描异常", fmt.Sprintf("%v", r), nil)
		}
	}()
	e.doTick(ctx)
}

// Stop 停止扫描循环。
func (e *Engine) Stop() { close(e.stop) }

// doTick 一轮扫描：补算缺失 next_run_at → 取到期规则 → 逐条触发。
func (e *Engine) doTick(ctx context.Context) {
	now := time.Now()

	// next_run_at 缺失的启用规则（直插 DB / 历史数据）补算下次触发时刻
	var unscheduled []model.TaskRule
	e.db.Where("enabled = ? AND next_run_at IS NULL", true).Limit(50).Find(&unscheduled)
	for i := range unscheduled {
		if next := e.computeNext(&unscheduled[i], now); next != nil {
			e.db.Model(&unscheduled[i]).Update("next_run_at", next)
		} else {
			e.db.Model(&unscheduled[i]).Update("enabled", false) // 触发值非法，禁用防反复扫描
		}
	}

	var rules []model.TaskRule
	if err := e.db.Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?",
		true, now).Limit(20).Find(&rules).Error; err != nil {
		return
	}
	for i := range rules {
		e.fire(ctx, &rules[i])
	}
}

// fire 触发单条规则：先推进 next_run_at 防重入，再执行。
func (e *Engine) fire(ctx context.Context, rule *model.TaskRule) {
	next := e.computeNext(rule, time.Now())
	e.db.Model(rule).Updates(map[string]interface{}{
		"last_run_at": time.Now(),
		"next_run_at": next,
		"enabled":     rule.TriggerType != "once", // once 执行后归档（禁用）
	})
	e.executeRule(ctx, rule)
}

// RunNow 立即执行一次规则（不新建规则、不影响调度时刻）。
func (e *Engine) RunNow(ctx context.Context, rule *model.TaskRule) {
	e.executeRule(ctx, rule)
}

// executeRule 对规则的目标账号逐个执行能力。
func (e *Engine) executeRule(ctx context.Context, rule *model.TaskRule) {
	accounts, err := e.selectAccounts(rule)
	if err != nil {
		e.recordRun(rule, nil, "failed", "", err.Error())
		return
	}

	for _, acct := range accounts {
		var run model.TaskRun
		run.RuleID = &rule.ID
		run.Status = "running"
		run.StartedAt = time.Now()
		e.db.Create(&run)

		req := &pb.RunTaskRequest{CapabilityId: rule.CapabilityID}
		if acct != nil {
			run.AccountID = &acct.ID
			req.Credential = account.BuildCred(e.db, e.dataDir, acct, 0)
		}

		resp, err := e.runner.RunTask(ctx, pluginNameByID(e.db, rule.PluginID), req)
		if err != nil {
			run.Status = "failed"
			run.ErrorMessage = truncate(err.Error(), 1000)
		} else if resp.Error != nil && resp.Error.Code != 0 {
			run.Status = "failed"
			run.ErrorMessage = truncate(resp.Error.Message, 1000)
		} else {
			run.Status = "success"
			run.Summary = truncate(resp.Summary, 1000)
			// 结构化明细快照（如成长任务列表）持久化，账号详情弹窗直接渲染
			if len(resp.DetailJson) > 0 {
				run.DetailJSON = truncate(resp.DetailJson, 1<<20)
			}
			// 凭据变更（如 token 刷新）回写账号
			if resp.Changed && acct != nil && len(resp.Blob) > 0 {
				e.db.Model(&model.Account{}).Where("id = ?", acct.ID).
					Updates(map[string]interface{}{
						"credential_blob": account.EncryptCredential(e.dataDir, resp.Blob),
						"last_refresh_at": time.Now(),
					})
			}
		}
		// 插件要求提醒用户（如站点签到需人工前往）→ 落站内通知，成功/失败均可携带
		if resp != nil && resp.Notification != nil && resp.Notification.Title != "" {
			e.db.Create(&model.Notification{
				Title: truncate(resp.Notification.Title, 256), Content: truncate(resp.Notification.Content, 4000),
				Level: orDefault(resp.Notification.Level, "info"), AccountID: run.AccountID,
			})
		}
		fin := time.Now()
		run.FinishedAt = &fin
		e.db.Save(&run)

		if run.Status == "success" && acct != nil && e.bus != nil {
			e.bus.Publish(event.Event{Topic: event.TopicTaskCompleted, AccountID: acct.ID})
		}
	}
}

// selectAccounts 按 target_scope 选出目标账号；返回 nil 元素表示"全局执行一次"。
// 任务与对话调度分离：disabled（停用调度）账号仍跑任务，只排除 expired（凭据失效）。
func (e *Engine) selectAccounts(rule *model.TaskRule) ([]*model.Account, error) {
	taskable := []string{"active", "disabled"}
	switch rule.TargetScope {
	case "all":
		var accts []model.Account
		if err := e.db.Where("plugin_id = ? AND status IN ?", rule.PluginID, taskable).Find(&accts).Error; err != nil {
			return nil, err
		}
		out := make([]*model.Account, len(accts))
		for i := range accts {
			out[i] = &accts[i]
		}
		return out, nil
	case "rotate":
		var acct model.Account
		if err := e.db.Where("plugin_id = ? AND status IN ?", rule.PluginID, taskable).
			Order("last_refresh_at IS NULL, last_refresh_at").First(&acct).Error; err != nil {
			return nil, err
		}
		return []*model.Account{&acct}, nil
	case "account_ids":
		var ids []int64
		if err := json.Unmarshal([]byte(rule.TargetJSON), &ids); err != nil {
			return nil, err
		}
		var accts []model.Account
		if err := e.db.Where("id IN ? AND plugin_id = ? AND status IN ?", ids, rule.PluginID, taskable).
			Find(&accts).Error; err != nil {
			return nil, err
		}
		out := make([]*model.Account, len(accts))
		for i := range accts {
			out[i] = &accts[i]
		}
		return out, nil
	default: // 全局能力：不绑定账号
		return []*model.Account{nil}, nil
	}
}

// computeNext 计算下次触发时刻。
func (e *Engine) computeNext(rule *model.TaskRule, from time.Time) *time.Time {
	var next time.Time
	switch rule.TriggerType {
	case "interval":
		d, err := time.ParseDuration(rule.TriggerValue)
		if err != nil {
			return nil
		}
		next = from.Add(d)
	case "daily":
		hh, mm := 9, 0
		if n, err := fmt.Sscanf(rule.TriggerValue, "%d:%d", &hh, &mm); err != nil || n != 2 {
			return nil
		}
		next = time.Date(from.Year(), from.Month(), from.Day(), hh, mm, 0, 0, from.Location())
		if !next.After(from) {
			next = next.Add(24 * time.Hour)
		}
		// 随机抖动：设定时刻之后延迟 0~jitter，错开多账号同刻打上游（每天各自随机）；管理端可配，0 = 关闭
		if jitter := setting.New(e.db).DailyJitter(); jitter > 0 {
			next = next.Add(time.Duration(rand.Int63n(int64(jitter))))
		}
	case "cron":
		next = nextCron(rule.TriggerValue, from)
		if next.IsZero() {
			return nil
		}
	case "once":
		t, err := time.Parse(time.RFC3339, rule.TriggerValue)
		if err != nil {
			return nil
		}
		next = t
	default:
		return nil
	}
	return &next
}

// recordRun 记录无账号上下文的失败。
func (e *Engine) recordRun(rule *model.TaskRule, acctID *int64, status, summary, errMsg string) {
	var run model.TaskRun
	run.RuleID = &rule.ID
	run.AccountID = acctID
	run.Status = status
	run.Summary = summary
	run.ErrorMessage = errMsg
	run.StartedAt = time.Now()
	fin := time.Now()
	run.FinishedAt = &fin
	e.db.Create(&run)
}

// pluginNameByID 从 plugins 表取插件名（失败返回空串，调用侧按不存在处理）。
func pluginNameByID(db *gorm.DB, id int64) string {
	var p model.Plugin
	if err := db.First(&p, id).Error; err != nil {
		return ""
	}
	return p.Name
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ScheduleOnce 创建一条立即执行的 once 规则（手动触发/失败重跑都用它）。
func (e *Engine) ScheduleOnce(pluginID int64, capabilityID string, accountID int64) error {
	scope, target := "all", "[]"
	if accountID > 0 {
		scope = "account_ids"
		target = fmt.Sprintf("[%d]", accountID)
	}
	rule := model.TaskRule{
		PluginID:     pluginID,
		CapabilityID: capabilityID,
		TriggerType:  "once",
		TriggerValue: time.Now().Format(time.RFC3339),
		TargetScope:  scope,
		TargetJSON:   target,
		NextRunAt:    ptrTime(time.Now()),
	}
	return e.db.Create(&rule).Error
}

// EnsureAccountRules 新账号建档后，按插件声明的账号级任务能力自动生成规则（按账号所属实例查询，
// 实例关闭的能力不建规则）。生成的规则默认停用，用户在任务页确认调度后再启用。
func (e *Engine) EnsureAccountRules(ctx context.Context, pluginName string, accountID int64) {
	var p model.Plugin
	if err := e.db.Where("name = ?", pluginName).First(&p).Error; err != nil {
		return
	}
	var acct model.Account
	e.db.Select("instance_id").First(&acct, accountID)
	caps, err := e.runner.ListCapabilities(ctx, pluginName, acct.InstanceID)
	if err != nil {
		return
	}
	target := fmt.Sprintf("[%d]", accountID)
	for _, c := range caps {
		if !c.PerAccount {
			continue
		}
		tt, tv := parseSchedule(c.DefaultSchedule)
		e.db.Create(&model.TaskRule{
			PluginID: p.ID, CapabilityID: c.Id,
			TriggerType: tt, TriggerValue: tv,
			TargetScope: "account_ids", TargetJSON: target,
			Auto: true, Enabled: false,
		})
	}
}

// parseSchedule 解析能力声明的 default_schedule：
// "daily 09:00" / "interval 6h" / "cron 0 9 * * *" / "once"；空或不合法回退每天 09:00。
func parseSchedule(def string) (string, string) {
	kind, value, _ := strings.Cut(strings.TrimSpace(def), " ")
	switch kind {
	case "daily":
		if value == "" {
			value = "09:00"
		}
		return "daily", value
	case "interval":
		if value == "" {
			value = "6h"
		}
		return "interval", value
	case "cron":
		if value == "" {
			return "daily", "09:00"
		}
		return "cron", value
	case "once":
		return "once", time.Now().Format(time.RFC3339) // 不带时刻默认立即
	}
	return "daily", "09:00"
}

func ptrTime(t time.Time) *time.Time { return &t }
