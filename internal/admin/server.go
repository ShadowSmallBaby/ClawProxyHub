// Package admin — 管理后台 API。管理员密码存 users 表（bcrypt），
// 首启经 /setup 引导设置；CPH_ADMIN_PASSWORD 仅作容器化引导注入。
package admin

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/task"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// Server 管理后台。
type Server struct {
	db             *gorm.DB
	accounts       *account.Service
	plugins        *plugin.Manager
	engine         *task.Engine
	settings       *setting.Store
	marketplaceURL string
}

// New 创建管理后台；表空且配置了 CPH_ADMIN_PASSWORD 时自动引导建号。
func New(db *gorm.DB, accounts *account.Service, plugins *plugin.Manager, engine *task.Engine, settings *setting.Store, marketplaceURL string) *Server {
	s := &Server{
		db: db, accounts: accounts, plugins: plugins, engine: engine,
		settings: settings, marketplaceURL: marketplaceURL,
	}
	// 市场地址初始化：未配置时落官方默认（config 默认 = env 覆盖或官方地址），
	// 用户后续可在系统设置改为自建市场；生效顺序：settings 配置 > config 默认 > 离线兜底
	settings.EnsureDefault(setting.KeyMarketplaceURL, marketplaceURL)
	s.ensureAdminSeed()
	return s
}

// authed 鉴权路由注册器：h() 注册的处理器统一套 s.auth，杜绝漏包鉴权。
type authed struct {
	mux *http.ServeMux
	s   *Server
}

func (a authed) h(pattern string, fn http.HandlerFunc) { a.mux.HandleFunc(pattern, a.s.auth(fn)) }

// Handler 管理路由：免鉴权引导 + 按资源分组的鉴权路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// 首启引导 + 登录签发（免鉴权）
	mux.HandleFunc("GET /admin/setup-status", s.setupStatus)
	mux.HandleFunc("POST /admin/setup", s.setup)
	mux.HandleFunc("POST /admin/login", s.login)

	r := authed{mux: mux, s: s}
	s.routeSession(r)
	s.routePlugins(r)
	s.routeAccounts(r)
	s.routeKeys(r)
	s.routeGroups(r)
	s.routeProxies(r)
	s.routeRoutes(r)
	s.routeTasks(r)
	s.routeOAuth(r)
	s.routeSettingsStats(r)
	return mux
}

// routeOAuth 第三方平台登录态托管（供插件换取上游 token）。
func (s *Server) routeOAuth(r authed) {
	r.h("GET /admin/oauth-credentials", s.listOAuth)
	r.h("POST /admin/oauth-credentials", s.createOAuth)
	r.h("PUT /admin/oauth-credentials/{id}", s.updateOAuth)
	r.h("DELETE /admin/oauth-credentials/{id}", s.deleteOAuth)
}

// routeSession 会话：改密 / 当前用户。
func (s *Server) routeSession(r authed) {
	r.h("POST /admin/password", s.changePassword)
	r.h("GET /admin/me", s.me)
}

// routePlugins 插件：概览 / 设置 / 分发。
func (s *Server) routePlugins(r authed) {
	r.h("GET /admin/plugins", s.listPlugins)
	r.h("GET /admin/plugins/{name}/auth-methods", s.authMethods)
	r.h("GET /admin/plugins/{name}/settings", s.pluginSettings)
	r.h("GET /admin/plugins/{name}/task-capabilities", s.pluginTaskCapabilities)
	r.h("PUT /admin/plugins/{name}/settings", s.putPluginSettings)
	r.h("GET /admin/plugins/marketplace", s.marketplace)
	r.h("POST /admin/plugins/install-market", s.installMarket)
	r.h("POST /admin/plugins/install-upload", s.installUpload)
	r.h("POST /admin/plugins/{name}/stop", s.stopPlugin)
	r.h("POST /admin/plugins/{name}/start", s.startPlugin)
	r.h("DELETE /admin/plugins/{name}", s.uninstallPlugin)
}

// routeAccounts 账号：登录 / 详情 / 模型 / 代理 / 调度 / 测试。
func (s *Server) routeAccounts(r authed) {
	r.h("POST /admin/accounts/login", s.submitLogin)
	r.h("GET /admin/accounts", s.listAccounts)
	r.h("GET /admin/accounts/{id}/detail", s.accountDetail)
	r.h("GET /admin/accounts/{id}/models", s.accountModels)
	r.h("PUT /admin/accounts/{id}/models", s.saveAccountModels)
	r.h("DELETE /admin/accounts/{id}", s.deleteAccount)
	r.h("POST /admin/accounts/{id}/refresh", s.refreshAccount)
	r.h("POST /admin/accounts/{id}/pause", s.pauseAccount)
	r.h("POST /admin/accounts/{id}/resume", s.resumeAccount)
	r.h("PUT /admin/accounts/{id}", s.updateAccount)
	r.h("GET /admin/accounts/{id}/proxies", s.listAccountProxies)
	r.h("PUT /admin/accounts/{id}/proxies", s.bindAccountProxies)
	r.h("POST /admin/accounts/{id}/test", s.testAccount)
}

// routeKeys 密钥：签发 / 回显 / 改名 / 路由授权。
func (s *Server) routeKeys(r authed) {
	r.h("GET /admin/keys", s.listKeys)
	r.h("GET /admin/keys/{id}/reveal", s.revealKey)
	r.h("POST /admin/keys", s.createKey)
	r.h("DELETE /admin/keys/{id}", s.deleteKey)
	r.h("POST /admin/keys/{id}/toggle", s.toggleKey)
	r.h("PUT /admin/keys/{id}", s.updateKey)
	r.h("PUT /admin/keys/{id}/routes", s.bindKeyRoutes)
}

// routeGroups 分组：增删 / 代理绑定。
func (s *Server) routeGroups(r authed) {
	r.h("GET /admin/groups", s.listGroups)
	r.h("POST /admin/groups", s.createGroup)
	r.h("DELETE /admin/groups/{id}", s.deleteGroup)
	r.h("PUT /admin/groups/{id}/proxies", s.bindGroupProxies)
	r.h("GET /admin/groups/{id}/proxies", s.listGroupProxies)
}

// routeProxies 出站代理增删改查 + 连通性测试。
func (s *Server) routeProxies(r authed) {
	r.h("GET /admin/proxies", s.listProxies)
	r.h("POST /admin/proxies", s.createProxy)
	r.h("PUT /admin/proxies/{id}", s.updateProxy)
	r.h("DELETE /admin/proxies/{id}", s.deleteProxy)
	r.h("POST /admin/proxies/{id}/test", s.testProxy)
}

// routeRoutes 路由增删改查。
func (s *Server) routeRoutes(r authed) {
	r.h("GET /admin/routes", s.listRoutes)
	r.h("POST /admin/routes", s.createRoute)
	r.h("PUT /admin/routes/{id}", s.updateRoute)
	r.h("DELETE /admin/routes/{id}", s.deleteRoute)
}

// routeTasks 任务调度规则与执行历史。
func (s *Server) routeTasks(r authed) {
	r.h("GET /admin/task-rules", s.listTaskRules)
	r.h("POST /admin/task-rules", s.createTaskRule)
	r.h("POST /admin/task-rules/{id}/toggle", s.toggleTaskRule)
	r.h("DELETE /admin/task-rules/{id}", s.deleteTaskRule)
	r.h("POST /admin/task-rules/{id}/run", s.runTaskRule)
	r.h("GET /admin/task-runs", s.listTaskRuns)
}

// routeSettingsStats 系统设置 / 日志 / 仪表盘统计。
func (s *Server) routeSettingsStats(r authed) {
	r.h("GET /admin/settings", s.getSettings)
	r.h("PUT /admin/settings", s.putSettings)
	r.h("GET /admin/logs", s.listLogs)
	r.h("GET /admin/stats", s.dashboardStats)
	r.h("GET /admin/stats/quota", s.dashboardQuota)
	r.h("GET /admin/stats/trend", s.dashboardTrend)
	r.h("GET /admin/version", s.coreVersion)
}

// listPlugins GET /admin/plugins — 已启动插件概览（含授权方式）。
func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
	type pluginView struct {
		ID          int64             `json:"id"`
		Name        string            `json:"name"`
		Label       string            `json:"label"` // 品牌名（关联字段统一显示它）
		Version     string            `json:"version"`
		Author      string            `json:"author"`
		Icon        string            `json:"icon"` // 包内相对路径（空 = 前端兜底）
		Capability  []string          `json:"capabilities"`
		AuthMethods []*authMethodView `json:"auth_methods"`
	}
	var out []pluginView
	for _, name := range s.plugins.Names() {
		inst, ok := s.plugins.Get(name)
		if !ok {
			continue
		}
		m := inst.Manifest
		v := pluginView{Name: m.Name, Label: brandName(m), Version: m.Version, Author: m.Author,
			Capability: m.Capabilities}
		// icon：以落盘文件为准（前端 <img> 直接引用，免鉴权静态端点）
		if _, ok := s.plugins.IconFile(m.Name); ok {
			v.Icon = "/assets/plugins/" + m.Name + "/icon"
		}
		var rec model.Plugin // DB id（建分组/规则时引用）
		if err := s.db.Where("name = ?", m.Name).First(&rec).Error; err == nil {
			v.ID = rec.ID
		}
		for _, am := range m.AuthMethods {
			v.AuthMethods = append(v.AuthMethods, viewAuthMethod(am))
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"plugins": out})
}

// authMethods GET /admin/plugins/{name}/auth-methods — 授权方式详情（渲染 tab + 表单）。
func (s *Server) authMethods(w http.ResponseWriter, r *http.Request) {
	methods, err := s.accounts.AuthMethods(r.PathValue("name"))
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
		return
	}
	var out []*authMethodView
	for _, m := range methods {
		out = append(out, viewAuthMethod(m))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"auth_methods": out})
}

// submitLogin POST /admin/accounts/login — 提交一步登录（首步或后续步）。
// body: {plugin, method_id, form: {..}, state: "<base64>"}
func (s *Server) submitLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugin   string            `json:"plugin"`
		MethodID string            `json:"method_id"`
		Form     map[string]string `json:"form"`
		State    string            `json:"state"`
	}
	if !readBody(w, r, &body) {
		return
	}
	var state []byte
	if body.State != "" {
		var err error
		state, err = base64.StdEncoding.DecodeString(body.State)
		if err != nil {
			http.Error(w, `{"error":"invalid state"}`, http.StatusBadRequest)
			return
		}
	}
	outcome, err := s.accounts.SubmitLogin(r.Context(), body.Plugin, body.MethodID, body.Form, state)
	if err != nil {
		// 业务错误（验证码错误/凭据格式/上游拒绝）用 400：401 专属管理员会话失效，
		// 前端见 401 会清 token 跳登录页
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	if outcome.Next != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"done": false, "next": viewNextStep(outcome.Next)})
		return
	}
	// 建档完成：按插件账号级任务能力自动生成规则（默认停用，任务页手动启用）
	if outcome.AccountID > 0 {
		s.engine.EnsureAccountRules(r.Context(), body.Plugin, outcome.AccountID)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"done": true, "account_id": outcome.AccountID})
}

// listAccounts GET /admin/accounts?plugin=stub
func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	pluginName := r.URL.Query().Get("plugin")
	var accts []model.Account
	q := s.db
	if pluginName != "" {
		var p model.Plugin
		if err := s.db.Where("name = ?", pluginName).First(&p).Error; err != nil {
			http.Error(w, `{"error":"unknown plugin"}`, http.StatusNotFound)
			return
		}
		q = q.Where("plugin_id = ?", p.ID)
	}
	if err := q.Order("id").Find(&accts).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	type acctView struct {
		ID          int64   `json:"id"`
		PluginID    int64   `json:"plugin_id"`
		GroupIDs    []int64 `json:"group_ids"`
		Name        string  `json:"display_name"`
		Status      string  `json:"status"`
		PauseReason string  `json:"pause_reason"`
		PausedUntil *string `json:"paused_until"`
		RefreshAt   *string `json:"last_refresh_at"`
		Credits     *struct {
			Remaining string `json:"remaining,omitempty"`
			Total     string `json:"total,omitempty"`
		} `json:"credits,omitempty"`
	}
	var out []acctView
	for _, a := range accts {
		v := acctView{ID: a.ID, PluginID: a.PluginID, GroupIDs: accountGroupIDs(s.db, a.ID), Name: a.DisplayName,
			Status: a.Status, PauseReason: a.PauseReason}
		if a.PausedUntil != nil {
			t := a.PausedUntil.Format("2006-01-02T15:04:05Z07:00")
			v.PausedUntil = &t
		}
		if a.LastRefreshAt != nil {
			t := a.LastRefreshAt.Format("2006-01-02T15:04:05Z07:00")
			v.RefreshAt = &t
		}
		// 积分列：credits_json 快照里的剩余/总（插件解析了才有，无则不渲染该列）
		if a.CreditsJSON != "" {
			var c struct {
				Total     string `json:"total"`
				Remaining string `json:"remaining"`
			}
			if json.Unmarshal([]byte(a.CreditsJSON), &c) == nil && (c.Total != "" || c.Remaining != "") {
				v.Credits = &struct {
					Remaining string `json:"remaining,omitempty"`
					Total     string `json:"total,omitempty"`
				}{Remaining: c.Remaining, Total: c.Total}
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"accounts": out})
}

// deleteAccount DELETE /admin/accounts/{id}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.accounts.Delete(parseInt(r.PathValue("id"))); err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// refreshAccount POST /admin/accounts/{id}/refresh
func (s *Server) refreshAccount(w http.ResponseWriter, r *http.Request) {
	acct, err := s.accounts.Refresh(r.Context(), parseInt(r.PathValue("id")))
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": acct.Status})
}

// ---------- 视图映射（proto → JSON，前端直接消费） ----------

type authFieldView struct {
	Name        string            `json:"name"`
	Label       map[string]string `json:"label"`
	Type        string            `json:"type"`
	Required    bool              `json:"required"`
	Placeholder string            `json:"placeholder"`
}

type authMethodView struct {
	ID           string            `json:"id"`
	Label        map[string]string `json:"label"`
	Fields       []authFieldView   `json:"fields"`
	Capabilities []string          `json:"capabilities"`
	Callback     string            `json:"callback,omitempty"` // auto / wait / auto_wait
}

type nextStepView struct {
	Action string            `json:"action"`
	URL    string            `json:"url,omitempty"`
	Prompt map[string]string `json:"prompt,omitempty"`
	Fields []authFieldView   `json:"fields,omitempty"`
	State  string            `json:"state,omitempty"`
	Wait   bool              `json:"wait,omitempty"`
}

func viewAuthMethod(m *pb.AuthMethod) *authMethodView {
	v := &authMethodView{ID: m.Id, Label: m.Label, Capabilities: m.Capabilities, Callback: m.Callback}
	for _, f := range m.Fields {
		v.Fields = append(v.Fields, viewAuthField(f))
	}
	return v
}

func viewAuthField(f *pb.AuthField) authFieldView {
	return authFieldView{
		Name: f.Name, Label: f.Label, Type: f.Type,
		Required: f.Required, Placeholder: f.Placeholder,
	}
}

func viewNextStep(n *pb.LoginNextStep) *nextStepView {
	v := &nextStepView{Action: n.Action, URL: n.Url, Prompt: n.Prompt, Wait: n.Wait}
	for _, f := range n.Fields {
		v.Fields = append(v.Fields, viewAuthField(f))
	}
	if len(n.State) > 0 {
		v.State = base64.StdEncoding.EncodeToString(n.State)
	}
	return v
}

// ---------- 工具 ----------

// brandName 插件品牌名：manifest.label.zh 优先，缺省用插件 id。
func brandName(m *pb.Manifest) string {
	if v, ok := m.Label["zh"]; ok && v != "" {
		return v
	}
	return m.Name
}

// pluginBrandByID 品牌名 by 插件 id（优先运行实例，回退 DB manifest 快照解析）。
func (s *Server) pluginBrandByID(pluginID int64) string {
	var p model.Plugin
	if err := s.db.First(&p, pluginID).Error; err != nil {
		return ""
	}
	if inst, ok := s.plugins.Get(p.Name); ok {
		return brandName(inst.Manifest)
	}
	var m pb.Manifest
	if err := protojson.Unmarshal([]byte(p.ManifestJSON), &m); err == nil {
		if v := m.Label["zh"]; v != "" {
			return v
		}
	}
	return p.Name
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return false
	}
	return true
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
