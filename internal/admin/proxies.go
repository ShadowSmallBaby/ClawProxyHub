// proxies.go — 出站代理管理与分组绑定。
package admin

import (
	"net/http"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// listProxies GET /admin/proxies
func (s *Server) listProxies(w http.ResponseWriter, r *http.Request) {
	var proxies []model.Proxy
	if err := s.db.Order("id").Find(&proxies).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proxies": proxies})
}

// createProxy POST /admin/proxies — body: {name, scheme, host, port, username, password}
func (s *Server) createProxy(w http.ResponseWriter, r *http.Request) {
	var body model.Proxy
	if !readBody(w, r, &body) || body.Host == "" || body.Port == 0 {
		http.Error(w, `{"error":"host and port required"}`, http.StatusBadRequest)
		return
	}
	if body.Scheme == "" {
		body.Scheme = "http"
	}
	if err := s.db.Create(&body).Error; err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": body.ID})
}

// deleteProxy DELETE /admin/proxies/{id}（分组绑定随之解除）
func (s *Server) deleteProxy(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	s.db.Where("proxy_id = ?", id).Delete(&model.GroupProxy{})
	s.db.Delete(&model.Proxy{}, id)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// bindGroupProxies PUT /admin/groups/{id}/proxies — body: {proxy_ids: []}，空 = 解除全部。
func (s *Server) bindGroupProxies(w http.ResponseWriter, r *http.Request) {
	gid := parseInt(r.PathValue("id"))
	var body struct {
		ProxyIDs []int64 `json:"proxy_ids"`
	}
	if !readBody(w, r, &body) {
		return
	}
	s.db.Where("group_id = ?", gid).Delete(&model.GroupProxy{})
	for _, pid := range body.ProxyIDs {
		s.db.Create(&model.GroupProxy{GroupID: gid, ProxyID: pid})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// listGroupProxies GET /admin/groups/{id}/proxies
func (s *Server) listGroupProxies(w http.ResponseWriter, r *http.Request) {
	gid := parseInt(r.PathValue("id"))
	var links []model.GroupProxy
	s.db.Where("group_id = ?", gid).Find(&links)
	ids := make([]int64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ProxyID)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proxy_ids": ids})
}
