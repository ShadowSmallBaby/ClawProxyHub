// proxies.go — 出站代理管理与分组绑定。
package admin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"

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

// updateProxy PUT /admin/proxies/{id} — body: {name, scheme, host, port, username, password}
// password 空字符串 = 保留原密码（前端不回显密码，避免误清空）。
func (s *Server) updateProxy(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var body model.Proxy
	if !readBody(w, r, &body) || body.Host == "" || body.Port == 0 {
		http.Error(w, `{"error":"host and port required"}`, http.StatusBadRequest)
		return
	}
	if body.Scheme == "" {
		body.Scheme = "http"
	}
	fields := map[string]interface{}{
		"name": body.Name, "scheme": body.Scheme, "host": body.Host,
		"port": body.Port, "username": body.Username,
	}
	if body.Password != "" {
		fields["password"] = body.Password
	}
	if err := s.db.Model(&model.Proxy{}).Where("id = ?", id).Updates(fields).Error; err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// deleteProxy DELETE /admin/proxies/{id}（分组绑定随之解除）
func (s *Server) deleteProxy(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	s.db.Where("proxy_id = ?", id).Delete(&model.GroupProxy{})
	s.db.Where("proxy_id = ?", id).Delete(&model.AccountProxy{})
	s.db.Delete(&model.Proxy{}, id)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// testProxy POST /admin/proxies/{id}/test — 经该代理拨到中立目标，验证连通性与时延。
// 已入库的代理直接读库；未落库的临时配置从 body 取（新建弹窗即时测试）。
func (s *Server) testProxy(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var px model.Proxy
	if id > 0 {
		if err := s.db.First(&px, id).Error; err != nil {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
	} else if !readBody(w, r, &px) || px.Host == "" || px.Port == 0 {
		http.Error(w, `{"error":"host and port required"}`, http.StatusBadRequest)
		return
	}
	start := time.Now()
	if err := probeProxy(r.Context(), &px); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "latency_ms": time.Since(start).Milliseconds()})
}

// probeProxy 经代理向中立目标发起一次 HTTP 请求，成功即代理可用。
// http/https 走 Transport.Proxy；socks5 用 x/net/proxy 拨号器接管连接建立。
func probeProxy(ctx context.Context, px *model.Proxy) error {
	const target = "https://www.gstatic.com/generate_204" // 全球可达、返回 204、无正文
	tr := &http.Transport{}
	switch px.Scheme {
	case "socks5":
		var auth *proxy.Auth
		if px.Username != "" {
			auth = &proxy.Auth{User: px.Username, Password: px.Password}
		}
		dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%d", px.Host, px.Port), auth, proxy.Direct)
		if err != nil {
			return fmt.Errorf("socks5 dialer: %w", err)
		}
		cd, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return fmt.Errorf("socks5 dialer not context-aware")
		}
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return cd.DialContext(ctx, network, addr)
		}
	default: // http / https
		u := &url.URL{Scheme: px.Scheme, Host: fmt.Sprintf("%s:%d", px.Host, px.Port)}
		if px.Username != "" {
			u.User = url.UserPassword(px.Username, px.Password)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return nil
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

// bindAccountProxies PUT /admin/accounts/{id}/proxies — body: {proxy_ids: []}，空 = 解除全部。
func (s *Server) bindAccountProxies(w http.ResponseWriter, r *http.Request) {
	aid := parseInt(r.PathValue("id"))
	var body struct {
		ProxyIDs []int64 `json:"proxy_ids"`
	}
	if !readBody(w, r, &body) {
		return
	}
	s.db.Where("account_id = ?", aid).Delete(&model.AccountProxy{})
	for _, pid := range body.ProxyIDs {
		s.db.Create(&model.AccountProxy{AccountID: aid, ProxyID: pid})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// listAccountProxies GET /admin/accounts/{id}/proxies
func (s *Server) listAccountProxies(w http.ResponseWriter, r *http.Request) {
	aid := parseInt(r.PathValue("id"))
	var links []model.AccountProxy
	s.db.Where("account_id = ?", aid).Find(&links)
	ids := make([]int64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ProxyID)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proxy_ids": ids})
}
