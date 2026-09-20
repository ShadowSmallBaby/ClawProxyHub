// keys.go — 密钥与分组、路由管理。
package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// keyMask 密钥掩码：id + 创建时间哈希前 4 字节（不泄露明文，仅用于识别）。
func keyMask(id int64, createdAt time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s", id, createdAt.Format("2006-01-02 15:04:05"))))
	return fmt.Sprintf("cph-****%s", hex.EncodeToString(h[:4]))
}

// listKeys GET /admin/keys — 密钥列表（含授权路由）。
func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	var keys []model.Key
	if err := s.db.Order("id").Find(&keys).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	// 最后调用时刻：request_logs 按 key 聚合
	type lastUse struct {
		KeyID   *int64
		MaxTime string
	}
	var lastUses []lastUse
	s.db.Model(&model.RequestLog{}).
		Select("key_id, MAX(created_at) AS max_time").
		Where("key_id IS NOT NULL").
		Group("key_id").Scan(&lastUses)
	lastUseMap := map[int64]string{}
	for _, lu := range lastUses {
		if lu.KeyID != nil {
			lastUseMap[*lu.KeyID] = lu.MaxTime
		}
	}

	type keyView struct {
		ID         int64   `json:"id"`
		Name       string  `json:"name"`
		Enabled    bool    `json:"enabled"`
		ExpiresAt  *string `json:"expires_at"`
		CreatedAt  string  `json:"created_at"`
		LastUsedAt string  `json:"last_used_at"` // 最后调用（空 = 从未）
		KeyMask    string  `json:"key_mask"`     // 掩码（cph-****abcd）
		RouteIDs   []int64 `json:"route_ids"`    // 空 = 全部路由
	}
	var out []keyView
	for _, k := range keys {
		v := keyView{ID: k.ID, Name: k.Name, Enabled: k.Enabled,
			CreatedAt:  k.CreatedAt.Format("2006-01-02 15:04:05"),
			LastUsedAt: lastUseMap[k.ID], KeyMask: keyMask(k.ID, k.CreatedAt)}
		if k.ExpiresAt != nil {
			t := k.ExpiresAt.Format("2006-01-02 15:04:05")
			v.ExpiresAt = &t
		}
		var routes []model.KeyRoute
		s.db.Where("key_id = ?", k.ID).Find(&routes)
		for _, kr := range routes {
			v.RouteIDs = append(v.RouteIDs, kr.RouteID)
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": out})
}

// revealKey GET /admin/keys/{id}/reveal — 单独回显密钥明文（供列表复制）。
func (s *Server) revealKey(w http.ResponseWriter, r *http.Request) {
	var k model.Key
	if err := s.db.First(&k, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	plain := account.DecryptCredential(s.accounts.DataDir(), []byte(k.KeyCipher))
	// 存量哈希（无 0x01 前缀）无法回显明文，提示重建
	if len(k.KeyCipher) == 64 && k.KeyCipher[0] != 0x01 {
		http.Error(w, `{"error":"legacy key stored hashed, please recreate"}`, http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"key": string(plain)})
}

// createKey POST /admin/keys — 创建密钥，明文只返回一次。
func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	readBody(w, r, &body)
	raw := "cph-" + randHex(24)
	k := model.Key{KeyCipher: string(account.EncryptCredential(s.accounts.DataDir(), []byte(raw))), Name: body.Name, Enabled: true}
	if err := s.db.Create(&k).Error; err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": k.ID, "key": raw})
}

// updateKey PUT /admin/keys/{id} — body: {name}，改名。
func (s *Server) updateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if err := s.db.Model(&model.Key{}).Where("id = ?", parseInt(r.PathValue("id"))).
		Update("name", body.Name).Error; err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteKey DELETE /admin/keys/{id}
func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	s.db.Where("key_id = ?", id).Delete(&model.KeyRoute{})
	s.db.Delete(&model.Key{}, id)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// toggleKey POST /admin/keys/{id}/toggle
func (s *Server) toggleKey(w http.ResponseWriter, r *http.Request) {
	var k model.Key
	if err := s.db.First(&k, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	s.db.Model(&k).Update("enabled", !k.Enabled)
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": !k.Enabled})
}

// bindKeyRoutes PUT /admin/keys/{id}/routes — body: {route_ids: []}，空 = 全部。
func (s *Server) bindKeyRoutes(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var body struct {
		RouteIDs []int64 `json:"route_ids"`
	}
	if !readBody(w, r, &body) {
		return
	}
	s.db.Where("key_id = ?", id).Delete(&model.KeyRoute{})
	for _, rid := range body.RouteIDs {
		s.db.Create(&model.KeyRoute{KeyID: id, RouteID: rid})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- 分组 ----------

// listGroups GET /admin/groups — 分组列表（含账号数）。
func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	var groups []model.Group
	if err := s.db.Order("id").Find(&groups).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	type groupView struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		PluginID    int64  `json:"plugin_id"`
		Plugin      string `json:"plugin"`
		PluginLabel string `json:"plugin_label"` // 品牌名
		Accounts    int64  `json:"accounts"`
	}
	var out []groupView
	for _, g := range groups {
		v := groupView{ID: g.ID, Name: g.Name, PluginID: g.PluginID}
		var p model.Plugin
		if err := s.db.First(&p, g.PluginID).Error; err == nil {
			v.Plugin = p.Name
		}
		v.PluginLabel = s.pluginBrandByID(g.PluginID)
		s.db.Model(&model.AccountGroup{}).Where("group_id = ?", g.ID).Count(&v.Accounts)
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"groups": out})
}

// createGroup POST /admin/groups — body: {name, plugin_id}
func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		PluginID int64  `json:"plugin_id"`
	}
	if !readBody(w, r, &body) || body.Name == "" || body.PluginID == 0 {
		http.Error(w, `{"error":"name and plugin_id required"}`, http.StatusBadRequest)
		return
	}
	g := model.Group{Name: body.Name, PluginID: body.PluginID}
	if err := s.db.Create(&g).Error; err != nil {
		http.Error(w, `{"error":"duplicate name"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": g.ID})
}

// deleteGroup DELETE /admin/groups/{id}
func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	s.db.Delete(&model.Group{}, parseInt(r.PathValue("id")))
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ---------- 路由 ----------

// listRoutes GET /admin/routes
func (s *Server) listRoutes(w http.ResponseWriter, r *http.Request) {
	var routes []model.Route
	if err := s.db.Order("id").Find(&routes).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"routes": routes})
}

// routeBody 创建/编辑路由共用的请求体。
type routeBody struct {
	Name            string                  `json:"name"`
	Strategy        string                  `json:"strategy"`
	Groups          []model.RouteGroupEntry `json:"groups"`
	TimeoutSeconds  int32                   `json:"timeout_seconds"`
	FailoverEnabled bool                    `json:"failover_enabled"`
	FailoverOn4xx   bool                    `json:"failover_on_4xx"`
	FailoverOn5xx   bool                    `json:"failover_on_5xx"`
	FailoverGroupID *int64                  `json:"failover_group_id"`
	FailoverModel   string                  `json:"failover_model"`
}

// validate 降级开启时必须配齐：触发状态类（4xx/5xx 至少一项）+ 降级分组 + 降级模型。
func (b *routeBody) validate() string {
	if b.Name == "" || len(b.Groups) == 0 {
		return "name and groups required"
	}
	if b.Strategy == "" {
		b.Strategy = "round_robin"
	}
	if b.TimeoutSeconds < 0 || b.TimeoutSeconds > 3600 {
		return "timeout_seconds 需在 0–3600 秒之间（0 = 跟随全局）"
	}
	if b.FailoverEnabled {
		if !b.FailoverOn4xx && !b.FailoverOn5xx {
			return "开启降级需至少勾选一种触发状态（4xx / 5xx）"
		}
		if b.FailoverGroupID == nil || *b.FailoverGroupID == 0 || b.FailoverModel == "" {
			return "开启降级需配置降级分组与降级模型"
		}
	}
	return ""
}

// createRoute POST /admin/routes
func (s *Server) createRoute(w http.ResponseWriter, r *http.Request) {
	var body routeBody
	if !readBody(w, r, &body) {
		return
	}
	if msg := body.validate(); msg != "" {
		http.Error(w, `{"error":"`+msg+`"}`, http.StatusBadRequest)
		return
	}
	groupsJSON, _ := json.Marshal(body.Groups)
	rt := model.Route{
		Name: body.Name, Strategy: body.Strategy, GroupsJSON: string(groupsJSON),
		TimeoutSeconds: body.TimeoutSeconds, FailoverEnabled: body.FailoverEnabled,
		FailoverOn4xx: body.FailoverOn4xx, FailoverOn5xx: body.FailoverOn5xx,
		FailoverGroupID: body.FailoverGroupID, FailoverModel: body.FailoverModel,
	}
	if err := s.db.Create(&rt).Error; err != nil {
		http.Error(w, `{"error":"duplicate name"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": rt.ID})
}

// updateRoute PUT /admin/routes/{id}
func (s *Server) updateRoute(w http.ResponseWriter, r *http.Request) {
	var rt model.Route
	if err := s.db.First(&rt, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	var body routeBody
	if !readBody(w, r, &body) {
		return
	}
	if msg := body.validate(); msg != "" {
		http.Error(w, `{"error":"`+msg+`"}`, http.StatusBadRequest)
		return
	}
	groupsJSON, _ := json.Marshal(body.Groups)
	rt.Name = body.Name
	rt.Strategy = body.Strategy
	rt.GroupsJSON = string(groupsJSON)
	rt.TimeoutSeconds = body.TimeoutSeconds
	rt.FailoverEnabled = body.FailoverEnabled
	rt.FailoverOn4xx = body.FailoverOn4xx
	rt.FailoverOn5xx = body.FailoverOn5xx
	rt.FailoverGroupID = body.FailoverGroupID
	rt.FailoverModel = body.FailoverModel
	if err := s.db.Save(&rt).Error; err != nil {
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteRoute DELETE /admin/routes/{id}
func (s *Server) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	s.db.Where("route_id = ?", id).Delete(&model.KeyRoute{})
	s.db.Delete(&model.Route{}, id)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ---------- 工具 ----------

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
