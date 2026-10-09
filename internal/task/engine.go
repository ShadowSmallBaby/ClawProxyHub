// engine.go — 调度引擎：周期扫描 + 有界 worker 异步执行（tick 不阻塞）。
package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/event"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/runlog"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/textutil"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// maxWorkers 限制同时执行的规则数。
const maxWorkers = 4

// Runner 是调度引擎对插件调用层的抽象（由 plugin.Manager 适配注入）。
type Runner interface {
	// ListCapabilities 返回插件声明的任务能力；instanceID>0 时插件可按实例配置裁剪。
	ListCapabilities(ctx context.Context, pluginName string, instanceID int64) ([]*pb.TaskCapability, error)
	// RunTask 触发一次能力执行。credential 为 nil 表示不针对具体账号。
	RunTask(ctx context.Context, pluginName string, req *pb.RunTaskRequest) (*pb.RunTaskResponse, error)
}

// Engine 周期扫描 task_rules，到期规则异步执行（tick 不阻塞）。
type Engine struct {
	owner      string
	external   bool
	active     map[int64]context.CancelFunc
	db         *gorm.DB
	dataDir    string
	runner     Runner
	bus        *event.Bus
	settings   *setting.Store
	scheduleMu sync.Mutex
	stop       chan struct{}
	stopped    sync.Once
	workers    sync.Once
	started    sync.Once
	wg         sync.WaitGroup
	waitOnce   sync.Once
	done       chan struct{}
	mu         sync.Mutex
	running    map[int64]bool
	queue      chan ruleJob
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewEngine 创建调度引擎。
func NewEngine(db *gorm.DB, dataDir string, runner Runner, bus *event.Bus, stores ...*setting.Store) *Engine {
	settings := setting.New(db)
	if len(stores) > 0 {
		settings = stores[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{owner: newToken(), active: map[int64]context.CancelFunc{}, ctx: ctx, cancel: cancel, running: map[int64]bool{}, queue: make(chan ruleJob, 64), db: db, dataDir: dataDir, runner: runner, bus: bus, settings: settings, stop: make(chan struct{}), done: make(chan struct{})}
}

// Start 启动扫描循环并恢复过期租约；其他宿主仍持有租约的任务继续执行。
func (e *Engine) Start(ctx context.Context) {
	e.started.Do(func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.ctx.Err() != nil {
			return
		}
		_ = e.RecoverExpired(time.Now().UTC())

		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			var tick <-chan time.Time
			if !e.external {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				tick = ticker.C
			}
			for {
				select {
				case <-ctx.Done():
					e.Stop()
					return
				case <-e.stop:
					return
				case <-tick:
					e.tick(ctx)
				}
			}
		}()
	})
}

// tick 执行一轮。单轮 panic 不退出进程，下一轮继续。
func (e *Engine) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[task] tick panicked: %v\n", r)
			runlog.New(e.db, e.settings.RunLevel).
				Error("task", "tick", "任务扫描异常", fmt.Sprintf("%v", r), nil)
		}
	}()
	e.doTick(ctx)
}

// Stop 幂等停止（二次调用安全）。
func (e *Engine) Stop() {
	e.stopped.Do(func() { close(e.stop); e.cancel() })
}

// Shutdown 等待扫描与执行 goroutine 退出，避免宿主关闭数据库后仍有任务写入。
func (e *Engine) Shutdown(ctx context.Context) error {
	e.Stop()
	e.mu.Lock()
	e.waitOnce.Do(func() { go func() { e.wg.Wait(); close(e.done) }() })
	e.mu.Unlock()
	select {
	case <-e.done:
		return e.releaseOwned()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// doTick 一轮扫描：补算缺失 next_run_at → 取到期规则 → 逐条异步触发。
func (e *Engine) doTick(ctx context.Context) {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	now := time.Now().UTC()
	if err := e.RecoverExpired(now); err != nil {
		return
	}

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
	// 队列限制执行并发；扫描不能只取前几条，否则无可用账号的规则会饿死后续任务。
	if err := e.db.Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?",
		true, now).Order("next_run_at, id").Find(&rules).Error; err != nil {
		return
	}
	for i := range rules {
		e.fire(ctx, &rules[i])
	}
}

// SaveSettings 与调度扫描串行，避免旧时区计算覆盖刚失效的日历规则。
func (e *Engine) SaveSettings(values map[string]string) error {
	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()
	return e.settings.SetMany(values)
}

// fire 入队成功才推进调度；队列满或同规则运行中则下轮再试。
func (e *Engine) fire(ctx context.Context, rule *model.TaskRule) {
	_, _ = e.enqueueContext(ctx, rule, true)
}
func (e *Engine) RunNow(ctx context.Context, rule *model.TaskRule) (int64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return e.enqueue(rule, false)
}

type ruleJob struct {
	token         string
	ctx           context.Context
	cancel        context.CancelFunc
	heartbeatDone chan struct{}
	rule          model.TaskRule
	accounts      []*model.Account
	runs          []model.TaskRun
}

func (e *Engine) enqueue(rule *model.TaskRule, scheduled bool) (int64, error) {
	return e.enqueueContext(e.ctx, rule, scheduled)
}
func (e *Engine) enqueueContext(trigger context.Context, rule *model.TaskRule, scheduled bool) (int64, error) {
	if err := trigger.Err(); err != nil {
		return 0, err
	}
	if err := ValidateRule(rule); err != nil {
		return 0, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx.Err() != nil {
		return 0, e.ctx.Err()
	}
	if e.running[rule.ID] {
		return 0, fmt.Errorf("rule is already queued or running")
	}
	if len(e.queue) == cap(e.queue) {
		return 0, fmt.Errorf("task queue is full")
	}
	accounts, err := e.selectAccounts(rule)
	if err != nil {
		return 0, err
	}
	if len(accounts) == 0 {
		return 0, fmt.Errorf("no eligible accounts")
	}
	job := ruleJob{rule: *rule, accounts: accounts, token: newToken()}
	now := time.Now().UTC()
	var next *time.Time
	if scheduled {
		next = e.computeNext(rule, now)
	}
	err = e.db.Transaction(func(tx *gorm.DB) error {
		claim := tx.Exec(`INSERT INTO task_claims(rule_id,token,owner,lease_until) VALUES(?,?,?,?) ON CONFLICT(rule_id) DO NOTHING`, rule.ID, job.token, e.owner, now.Add(claimLease).UnixMilli())
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return fmt.Errorf("rule is already claimed")
		}
		if scheduled {
			res := tx.Model(&model.TaskRule{}).Where("id = ? AND enabled = ? AND next_run_at = ? AND next_run_at <= ?", rule.ID, true, rule.NextRunAt, now).Updates(map[string]interface{}{"last_run_at": now, "next_run_at": next, "enabled": rule.TriggerType != "once"})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return fmt.Errorf("rule no longer enabled")
			}
		}
		for _, acct := range accounts {
			run := model.TaskRun{RuleID: &job.rule.ID, Status: "queued", StartedAt: now, ExecutionToken: job.token}
			if acct != nil {
				run.AccountID = &acct.ID
			}
			if err := tx.Create(&run).Error; err != nil {
				return err
			}
			job.runs = append(job.runs, run)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	e.running[rule.ID] = true
	job.ctx, job.cancel = context.WithCancel(e.ctx)
	cancelJob := job.cancel
	stopTrigger := context.AfterFunc(trigger, cancelJob)
	job.cancel = func() { stopTrigger(); cancelJob() }
	job.heartbeatDone = make(chan struct{})
	e.active[rule.ID] = job.cancel
	e.wg.Add(1)
	go func() { defer e.wg.Done(); defer close(job.heartbeatDone); e.heartbeat(job.ctx, job.token, job.cancel) }()
	e.workers.Do(func() {
		for i := 0; i < maxWorkers; i++ {
			e.wg.Add(1)
			go func() { defer e.wg.Done(); e.worker() }()
		}
	})
	e.queue <- job
	return job.runs[0].ID, nil
}
func (e *Engine) worker() {
	for {
		select {
		case <-e.ctx.Done():
			return
		case job := <-e.queue:
			for i, acct := range job.accounts {
				if job.ctx.Err() != nil || !e.hasClaim(job.token) {
					break
				}
				e.executeAccount(job.ctx, &job.rule, acct, &job.runs[i])
			}
			job.cancel()
			<-job.heartbeatDone
			_ = e.finishClaim(job.token, "cancelled before execution")
			e.mu.Lock()
			delete(e.running, job.rule.ID)
			delete(e.active, job.rule.ID)
			e.mu.Unlock()
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
	case "global":
		return []*model.Account{nil}, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

// computeNext 计算下次触发时刻。
func (e *Engine) computeNext(rule *model.TaskRule, from time.Time) *time.Time {
	var next time.Time
	switch rule.TriggerType {
	case "interval":
		d, err := time.ParseDuration(rule.TriggerValue)
		if err != nil || d <= 0 {
			return nil
		}
		next = from.Add(d)
	case "daily":
		clock, err := time.Parse("15:04", rule.TriggerValue)
		if err != nil {
			return nil
		}
		// 按当地日历逐分钟查找，夏令时跳过不存在的时刻，回拨时按实际时间排序。
		next = nextCron(fmt.Sprintf("%d %d * * *", clock.Minute(), clock.Hour()), from.In(e.settings.Location()))
		if next.IsZero() {
			return nil
		}
		// 随机抖动：设定时刻之后延迟 0~jitter，错开多账号同刻打上游（每天各自随机）；管理端可配，0 = 关闭
		if jitter := e.settings.DailyJitter(); jitter > 0 {
			next = next.Add(time.Duration(mathrand.Int63n(int64(jitter))))
		}
	case "cron":
		next = nextCron(rule.TriggerValue, from.In(e.settings.Location()))
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
	next = next.UTC()
	return &next
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
	return textutil.Truncate(s, n)
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
		Enabled:      true,
		PluginID:     pluginID,
		CapabilityID: capabilityID,
		TriggerType:  "once",
		TriggerValue: time.Now().UTC().Format(time.RFC3339),
		TargetScope:  scope,
		TargetJSON:   target,
		NextRunAt:    ptrTime(time.Now().UTC()),
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
		return "once", time.Now().UTC().Format(time.RFC3339) // 不带时刻默认立即
	}
	return "daily", "09:00"
}

func ptrTime(t time.Time) *time.Time { return &t }

func newToken() string {
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}
