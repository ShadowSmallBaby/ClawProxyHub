// instances.go — 实例（插件下的站点/部署）增删改查。
// 账号强制归属实例；单例插件的默认实例不可删，删实例级联清掉其下分组/账号/任务。
package admin

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// routeInstances 实例 CRUD。
func (s *Server) routeInstances(r authed) {
	r.h("GET /admin/instances", s.listInstances)
	r.h("POST /admin/instances", s.createInstance)
	r.h("PUT /admin/instances/{id}", s.updateInstance)
	r.h("DELETE /admin/instances/{id}", s.deleteInstance)
	r.h("GET /admin/instances/{id}/impact", s.instanceImpact)
}

type instanceView struct {
	ID           int64           `json:"id"`
	PluginID     int64           `json:"plugin_id"`
	Name         string          `json:"name"`
	BaseURL      string          `json:"base_url"`
	Settings     json.RawMessage `json:"settings"`
	AccountCount int64           `json:"account_count"`
}

func (s *Server) viewInstance(i *model.Instance) instanceView {
	var n int64
	s.db.Model(&model.Account{}).Where("instance_id = ?", i.ID).Count(&n)
	return instanceView{ID: i.ID, PluginID: i.PluginID, Name: i.Name, BaseURL: i.BaseURL,
		Settings: jsonOrNull(i.SettingsJSON), AccountCount: n}
}

// multiInstance 插件（按 DB id）是否声明多实例能力；未运行按单例处理。
func (s *Server) multiInstance(pluginID int64) bool {
	var p model.Plugin
	if err := s.db.Select("name").First(&p, pluginID).Error; err != nil {
		return false
	}
	inst, ok := s.plugins.Get(p.Name)
	return ok && inst.MultiInstance()
}

// listInstances GET /admin/instances?plugin_id= — 单例插件保证默认实例存在；多实例插件由用户手动创建。
func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	q := s.db.Order("plugin_id, id")
	if pid := parseInt(r.URL.Query().Get("plugin_id")); pid > 0 {
		if !s.multiInstance(pid) {
			account.DefaultInstance(s.db, pid)
		}
		q = q.Where("plugin_id = ?", pid)
	}
	var list []model.Instance
	if err := q.Find(&list).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	out := make([]instanceView, 0, len(list))
	for i := range list {
		out = append(out, s.viewInstance(&list[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"instances": out})
}

type instanceBody struct {
	PluginID int64           `json:"plugin_id"`
	Name     string          `json:"name"`
	BaseURL  string          `json:"base_url"`
	Settings json.RawMessage `json:"settings"`
}

// validHost 校验 URL 语义 host：域名 / IPv4 / IPv6 / localhost / 短主机名（自托管内网合法）。
func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 || strings.Trim(host, "0123456789.") == "" {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// validBaseURL 允许内网、IPv6 与基础路径，禁止 URL 内夹带凭据、查询和片段。
func validBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !validHost(u.Hostname()) || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

// normalize 清洗输入：名称必填、base_url 解析校验、settings 须为 JSON 对象。
func (b *instanceBody) normalize() (string, bool) {
	b.Name = strings.TrimSpace(b.Name)
	b.BaseURL = strings.TrimRight(strings.TrimSpace(b.BaseURL), "/")
	if b.Name == "" {
		return "name required", false
	}
	if !validBaseURL(b.BaseURL) {
		return "base_url required (http:// or https:// with valid host)", false
	}
	if len(b.Settings) == 0 {
		b.Settings = json.RawMessage("{}")
	}
	var probe map[string]interface{}
	if json.Unmarshal(b.Settings, &probe) != nil {
		return "settings must be a JSON object", false
	}
	return "", true
}

// createInstance POST /admin/instances — body: {plugin_id, name, base_url, settings}
func (s *Server) createInstance(w http.ResponseWriter, r *http.Request) {
	var body instanceBody
	if !readBody(w, r, &body) {
		return
	}
	if msg, ok := body.normalize(); !ok {
		http.Error(w, `{"error":"`+msg+`"}`, http.StatusBadRequest)
		return
	}
	var p model.Plugin
	if err := s.db.First(&p, body.PluginID).Error; err != nil {
		http.Error(w, `{"error":"plugin not found"}`, http.StatusNotFound)
		return
	}
	// 未声明多实例能力的插件只有默认实例（可编辑不可新增）
	if !s.multiInstance(p.ID) {
		http.Error(w, `{"error":"该插件不支持多实例（未声明 instances 能力或契约版本过旧）"}`, http.StatusBadRequest)
		return
	}
	inst := model.Instance{PluginID: p.ID, Name: body.Name, BaseURL: body.BaseURL, SettingsJSON: string(body.Settings)}
	if err := s.db.Create(&inst).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.viewInstance(&inst))
}

// updateInstance PUT /admin/instances/{id} — body 同 create（plugin_id 忽略，不可改归属）。
func (s *Server) updateInstance(w http.ResponseWriter, r *http.Request) {
	var body instanceBody
	if !readBody(w, r, &body) {
		return
	}
	if msg, ok := body.normalize(); !ok {
		http.Error(w, `{"error":"`+msg+`"}`, http.StatusBadRequest)
		return
	}
	var inst model.Instance
	if err := s.db.First(&inst, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	s.db.Model(&inst).Updates(map[string]interface{}{
		"name": body.Name, "base_url": body.BaseURL, "settings_json": string(body.Settings),
	})
	s.db.First(&inst, inst.ID)
	writeJSON(w, http.StatusOK, s.viewInstance(&inst))
}

// deleteInstance DELETE /admin/instances/{id} — 级联删除分组/账号/任务；单例插件须保留其默认实例。
func (s *Server) deleteInstance(w http.ResponseWriter, r *http.Request) {
	var inst model.Instance
	if err := s.db.First(&inst, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if !s.multiInstance(inst.PluginID) {
		http.Error(w, `{"error":"单例插件的默认实例不可删除"}`, http.StatusConflict)
		return
	}
	sc := scopeInstance(s.db, inst.ID)
	impact := s.impact(sc)
	if err := s.cascadeDelete(sc); err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "impact": impact})
}
