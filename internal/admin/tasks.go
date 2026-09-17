// tasks.go — 任务规则与执行历史。
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// pluginNameByID 插件 id → name。
func pluginNameByID(db *gorm.DB, id int64) string {
	var p model.Plugin
	if err := db.First(&p, id).Error; err != nil {
		return ""
	}
	return p.Name
}

// capabilityLabelMap 运行中插件的能力 id → 展示名（zh 优先，回退 en / id）。
func (s *Server) capabilityLabelMap(pluginName string) map[string]string {
	inst, ok := s.plugins.Get(pluginName)
	if !ok {
		return nil
	}
	resp, err := inst.Client().ListTaskCapabilities(context.Background(), &pb.Empty{})
	if err != nil {
		return nil
	}
	m := map[string]string{}
	for _, c := range resp.Capabilities {
		label := c.Label["zh"]
		if label == "" {
			label = c.Label["en"]
		}
		if label == "" {
			label = c.Id
		}
		m[c.Id] = label
	}
	return m
}

// ruleView 规则视图：能力展示名 + 插件品牌 + 账号范围，不暴露业务 id。
type ruleView struct {
	ID           int64      `json:"id"`
	PluginID     int64      `json:"plugin_id"`
	Plugin       string     `json:"plugin"`
	CapabilityID string     `json:"capability_id"`
	Capability   string     `json:"capability"`
	TriggerType  string     `json:"trigger_type"`
	TriggerValue string     `json:"trigger_value"`
	TargetScope  string     `json:"target_scope"`
	Accounts     []string   `json:"accounts"` // account_ids 范围下的账号名
	Enabled      bool       `json:"enabled"`
	NextRunAt    *time.Time `json:"next_run_at"`
	LastRunAt    *time.Time `json:"last_run_at"`
}

// accountNamesByID 按 id 列表取账号展示名。
func (s *Server) accountNamesByID(ids []int64) []string {
	var accts []model.Account
	if len(ids) > 0 {
		s.db.Where("id IN ?", ids).Find(&accts)
	}
	out := make([]string, 0, len(accts))
	for _, a := range accts {
		out = append(out, a.DisplayName)
	}
	return out
}

// listTaskRules GET /admin/task-rules
func (s *Server) listTaskRules(w http.ResponseWriter, r *http.Request) {
	var rules []model.TaskRule
	if err := s.db.Order("id").Find(&rules).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	labelCache := map[string]map[string]string{} // pluginName → labels
	var out []ruleView
	for _, rule := range rules {
		pluginName := pluginNameByID(s.db, rule.PluginID)
		labels, ok := labelCache[pluginName]
		if !ok {
			labels = s.capabilityLabelMap(pluginName)
			labelCache[pluginName] = labels
		}
		cap := rule.CapabilityID
		if labels != nil && labels[cap] != "" {
			cap = labels[cap]
		}
		// 账号范围展示：account_ids 解析 TargetJSON；其余 scope 给中文说明
		var accounts []string
		switch rule.TargetScope {
		case "all":
			accounts = []string{"全部账号"}
		case "rotate":
			accounts = []string{"轮换单账号"}
		case "account_ids":
			var ids []int64
			_ = json.Unmarshal([]byte(rule.TargetJSON), &ids)
			accounts = s.accountNamesByID(ids)
		}
		out = append(out, ruleView{
			ID: rule.ID, PluginID: rule.PluginID, Plugin: s.pluginBrandByID(rule.PluginID),
			CapabilityID: rule.CapabilityID, Capability: cap,
			TriggerType: rule.TriggerType, TriggerValue: rule.TriggerValue,
			TargetScope: rule.TargetScope, Accounts: accounts,
			Enabled: rule.Enabled, NextRunAt: rule.NextRunAt, LastRunAt: rule.LastRunAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rules": out})
}

// pluginTaskCapabilities GET /admin/plugins/{name}/task-capabilities — 新建规则弹窗的能力下拉数据。
func (s *Server) pluginTaskCapabilities(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	inst, ok := s.plugins.Get(name)
	if !ok {
		http.Error(w, `{"error":"plugin not found"}`, http.StatusNotFound)
		return
	}
	resp, err := inst.Client().ListTaskCapabilities(context.Background(), &pb.Empty{})
	if err != nil {
		http.Error(w, `{"error":"list capabilities failed"}`, http.StatusInternalServerError)
		return
	}
	type capItem struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	out := make([]capItem, 0, len(resp.Capabilities))
	for _, c := range resp.Capabilities {
		label := c.Label["zh"]
		if label == "" {
			label = c.Label["en"]
		}
		if label == "" {
			label = c.Id
		}
		out = append(out, capItem{ID: c.Id, Label: label})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"capabilities": out})
}

// createTaskRule POST /admin/task-rules
// body: {plugin_id, capability_id, trigger_type, trigger_value, target_scope}
func (s *Server) createTaskRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PluginID     int64  `json:"plugin_id"`
		CapabilityID string `json:"capability_id"`
		TriggerType  string `json:"trigger_type"`
		TriggerValue string `json:"trigger_value"`
		TargetScope  string `json:"target_scope"`
	}
	if !readBody(w, r, &body) || body.PluginID == 0 || body.CapabilityID == "" {
		http.Error(w, `{"error":"plugin_id and capability_id required"}`, http.StatusBadRequest)
		return
	}
	if body.TargetScope == "" {
		body.TargetScope = "all"
	}
	rule := model.TaskRule{
		PluginID: body.PluginID, CapabilityID: body.CapabilityID,
		TriggerType: body.TriggerType, TriggerValue: body.TriggerValue,
		TargetScope: body.TargetScope, TargetJSON: "[]", Enabled: true,
	}
	// next_run_at 由引擎 tick 补算
	if err := s.db.Create(&rule).Error; err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": rule.ID})
}

// deleteTaskRule DELETE /admin/task-rules/{id}
func (s *Server) deleteTaskRule(w http.ResponseWriter, r *http.Request) {
	s.db.Delete(&model.TaskRule{}, parseInt(r.PathValue("id")))
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// toggleTaskRule POST /admin/task-rules/{id}/toggle — 启用/停用（自动生成的规则默认停用，在此启用）。
func (s *Server) toggleTaskRule(w http.ResponseWriter, r *http.Request) {
	var rule model.TaskRule
	if err := s.db.First(&rule, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	s.db.Model(&rule).Update("enabled", !rule.Enabled)
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": !rule.Enabled})
}

// runTaskRule POST /admin/task-rules/{id}/run — 手动触发一次。
func (s *Server) runTaskRule(w http.ResponseWriter, r *http.Request) {
	var rule model.TaskRule
	if err := s.db.First(&rule, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	// 引擎实例由 main 注入（可选）
	if s.engine == nil {
		http.Error(w, `{"error":"engine not available"}`, http.StatusServiceUnavailable)
		return
	}
	// 直接执行该规则（不新建 once 规则、不影响调度时刻）
	s.engine.RunNow(r.Context(), &rule)
	writeJSON(w, http.StatusOK, map[string]bool{"scheduled": true})
}

// runView 执行历史的语义视图：不暴露规则/账号/插件的业务 id。
type runView struct {
	ID           int64           `json:"id"`
	Plugin       string          `json:"plugin"`
	Capability   string          `json:"capability"`
	Account      string          `json:"account"`
	Status       string          `json:"status"`
	Summary      string          `json:"summary"`
	Detail       json.RawMessage `json:"detail,omitempty"` // 结构化明细快照（如成长任务列表）
	ErrorMessage string          `json:"error_message"`
	StartedAt    time.Time       `json:"started_at"`
	FinishedAt   *time.Time      `json:"finished_at"`
}

// runViews 批量组装语义视图（规则/账号带小缓存查名字）。
func (s *Server) runViews(runs []model.TaskRun) []runView {
	ruleCache := map[int64]model.TaskRule{}
	acctCache := map[int64]model.Account{}
	var out []runView
	for _, run := range runs {
		v := runView{ID: run.ID, Status: run.Status, Summary: run.Summary,
			ErrorMessage: run.ErrorMessage, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt}
		if run.DetailJSON != "" {
			v.Detail = json.RawMessage(run.DetailJSON)
		}
		if run.RuleID != nil {
			rule, ok := ruleCache[*run.RuleID]
			if !ok {
				s.db.First(&rule, *run.RuleID)
				ruleCache[*run.RuleID] = rule
			}
			if rule.ID != 0 {
				v.Capability = rule.CapabilityID
				if labels := s.capabilityLabelMap(pluginNameByID(s.db, rule.PluginID)); labels != nil && labels[rule.CapabilityID] != "" {
					v.Capability = labels[rule.CapabilityID]
				}
				v.Plugin = s.pluginBrandByID(rule.PluginID)
			}
		}
		if run.AccountID != nil {
			acct, ok := acctCache[*run.AccountID]
			if !ok {
				s.db.First(&acct, *run.AccountID)
				acctCache[*run.AccountID] = acct
			}
			if acct.ID != 0 {
				v.Account = acct.DisplayName
			}
		}
		out = append(out, v)
	}
	return out
}

// listTaskRuns GET /admin/task-runs?limit=50
func (s *Server) listTaskRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if n := parseInt(r.URL.Query().Get("limit")); n > 0 && n <= 500 {
		limit = int(n)
	}
	var runs []model.TaskRun
	if err := s.db.Order("id DESC").Limit(limit).Find(&runs).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": s.runViews(runs)})
}

// dashboardTrend GET /admin/stats/trend?days=7 — 请求量/成功/tokens 按天聚合。
func (s *Server) dashboardTrend(w http.ResponseWriter, r *http.Request) {
	days := 7
	if n := parseInt(r.URL.Query().Get("days")); n > 0 && n <= 90 {
		days = int(n)
	}
	type point struct {
		Date     string `json:"date"`
		Requests int64  `json:"requests"`
		Success  int64  `json:"success"`
		Tokens   int64  `json:"tokens"`
	}
	var points []point
	s.db.Raw(`SELECT date(created_at) AS date, COUNT(*) AS requests,
		SUM(CASE WHEN status < 400 THEN 1 ELSE 0 END) AS success,
		COALESCE(SUM(input_tokens + output_tokens), 0) AS tokens
		FROM request_logs WHERE created_at >= date('now', ?)
		GROUP BY date(created_at) ORDER BY date`, fmt.Sprintf("-%d days", days)).Scan(&points)
	writeJSON(w, http.StatusOK, map[string]interface{}{"trend": points})
}

// ---------- 请求日志 / 概览 ----------

// listLogs GET /admin/logs?limit=100&key_id= — 调用日志（key_name 由 keys 表聚合）。
func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if n := parseInt(r.URL.Query().Get("limit")); n > 0 && n <= 1000 {
		limit = int(n)
	}
	q := s.db.Model(&model.RequestLog{})
	if kid := parseInt(r.URL.Query().Get("key_id")); kid > 0 {
		q = q.Where("key_id = ?", kid)
	}
	var logs []model.RequestLog
	if err := q.Order("id DESC").Limit(limit).Find(&logs).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	// 密钥名称映射（id → name，日志展示用）
	type keyName struct {
		ID   int64
		Name string
	}
	var keys []keyName
	s.db.Model(&model.Key{}).Select("id, name").Scan(&keys)
	keyNames := map[int64]string{}
	for _, k := range keys {
		keyNames[k.ID] = k.Name
	}
	type logView struct {
		model.RequestLog
		KeyName string `json:"key_name"` // 密钥名称（空 = 匿名/无密钥）
	}
	out := make([]logView, 0, len(logs))
	for _, l := range logs {
		v := logView{RequestLog: l}
		if l.KeyID != nil {
			v.KeyName = keyNames[*l.KeyID]
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"logs": out})
}

// dashboardQuota GET /admin/stats/quota — 按插件聚合账号积分快照（credits_json，
// 插件解析上游后持久化）与 profile.quota 兜底；仅对可解析为数字的值求和，无数据的插件不返回。
func (s *Server) dashboardQuota(w http.ResponseWriter, r *http.Request) {
	type acctRow struct {
		PluginName  string `json:"plugin_name"`
		CreditsJSON string `json:"credits_json"`
		ProfileJSON string `json:"profile_json"`
	}
	var rows []acctRow
	s.db.Model(&model.Account{}).
		Select("plugins.name AS plugin_name, accounts.credits_json AS credits_json, accounts.profile_json AS profile_json").
		Joins("JOIN plugins ON plugins.id = accounts.plugin_id").
		Scan(&rows)

	type pluginQuota struct {
		Plugin    string             `json:"plugin"`
		Accounts  int                `json:"accounts"`
		WithQuota int                `json:"with_quota"`
		Quota     map[string]float64 `json:"quota"`
	}
	// quota 标准键与 credits_json 字段对齐：credits=剩余 / total_credits=总 / used_credits=已用
	quotaKeys := [][2]string{
		{"credits", "remaining"},
		{"total_credits", "total"},
		{"used_credits", "used"},
	}
	byPlugin := map[string]*pluginQuota{}
	var order []string
	for _, row := range rows {
		pq, ok := byPlugin[row.PluginName]
		if !ok {
			pq = &pluginQuota{Plugin: row.PluginName, Quota: map[string]float64{}}
			byPlugin[row.PluginName] = pq
			order = append(order, row.PluginName)
		}
		pq.Accounts++

		add := func(key string, v float64) {
			pq.Quota[key] += v
		}
		hasNumeric := false

		// 优先：积分明细快照（真实上游数据，含 packages）
		if row.CreditsJSON != "" {
			var credits struct {
				Total     string `json:"total"`
				Used      string `json:"used"`
				Remaining string `json:"remaining"`
			}
			if json.Unmarshal([]byte(row.CreditsJSON), &credits) == nil {
				pairs := map[string]string{
					"credits": credits.Remaining, "total_credits": credits.Total, "used_credits": credits.Used,
				}
				for k, raw := range pairs {
					if raw != "" {
						if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
							add(k, v)
							hasNumeric = true
						}
					}
				}
			}
		}
		// 兜底：profile.quota（插件未产明细快照时）
		if !hasNumeric {
			var profile struct {
				Quota map[string]string `json:"quota"`
			}
			if json.Unmarshal([]byte(row.ProfileJSON), &profile) == nil {
				for _, kk := range quotaKeys {
					if raw, ok := profile.Quota[kk[0]]; ok && raw != "" {
						if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
							add(kk[0], v)
							hasNumeric = true
						}
					}
				}
			}
		}
		if hasNumeric {
			pq.WithQuota++
		}
	}
	out := make([]*pluginQuota, 0, len(order))
	for _, name := range order {
		out = append(out, byPlugin[name])
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"plugins": out})
}

// dashboardStats GET /admin/stats — 概览卡片数据。
func (s *Server) dashboardStats(w http.ResponseWriter, r *http.Request) {
	var stats struct {
		TotalRequests  int64 `json:"total_requests"`
		TodayRequests  int64 `json:"today_requests"`
		SuccessRate    int64 `json:"success_rate"` // 百分比
		TotalTokens    int64 `json:"total_tokens"`
		ActiveKeys     int64 `json:"active_keys"`
		ActiveAccounts int64 `json:"active_accounts"`
		RunningPlugins int64 `json:"running_plugins"`
	}
	s.db.Model(&model.RequestLog{}).Count(&stats.TotalRequests)
	s.db.Model(&model.RequestLog{}).Where("created_at >= date('now','localtime')").Count(&stats.TodayRequests)
	var okCount int64
	s.db.Model(&model.RequestLog{}).Where("status < 400").Count(&okCount)
	if stats.TotalRequests > 0 {
		stats.SuccessRate = okCount * 100 / stats.TotalRequests
	}
	s.db.Model(&model.RequestLog{}).Select("COALESCE(SUM(input_tokens+output_tokens),0)").Scan(&stats.TotalTokens)
	s.db.Model(&model.Key{}).Where("enabled = ?", true).Count(&stats.ActiveKeys)
	s.db.Model(&model.Account{}).Where("status = ?", "active").Count(&stats.ActiveAccounts)
	stats.RunningPlugins = int64(len(s.plugins.Names()))
	writeJSON(w, http.StatusOK, stats)
}
