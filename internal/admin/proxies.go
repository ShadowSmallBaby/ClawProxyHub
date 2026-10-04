// proxies.go — 出站代理管理与分组绑定。
package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// validProxyScheme 校验出站代理协议白名单。
func validProxyScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "socks5":
		return true
	}
	return false
}

// proxyBody 出站代理请求体（password 落库前加密，列表永不回显）。
type proxyBody struct {
	Name     string `json:"name"`
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// validate 校验协议白名单、端口范围与非空 host（域名/IP/短主机名/IPv6 均合法，允许内网）。
func (b *proxyBody) validate() bool {
	if b.Scheme == "" {
		b.Scheme = "http"
	}
	host := strings.TrimSpace(b.Host)
	if !validProxyScheme(b.Scheme) || host == "" || len(host) > 255 || b.Port < 1 || b.Port > 65535 {
		return false
	}
	return validHost(strings.Trim(host, "[]"))
}

// listProxies GET /admin/proxies — 白名单 DTO：密码只暴露"是否已设置"，不回明文/密文。
func (s *Server) listProxies(w http.ResponseWriter, r *http.Request) {
	var proxies []model.Proxy
	if err := s.db.Order("id").Find(&proxies).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	type proxyView struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Scheme      string `json:"scheme"`
		Host        string `json:"host"`
		Port        int32  `json:"port"`
		Username    string `json:"username"`
		HasPassword bool   `json:"has_password"`
	}
	out := make([]proxyView, 0, len(proxies))
	for _, px := range proxies {
		out = append(out, proxyView{ID: px.ID, Name: px.Name, Scheme: px.Scheme, Host: px.Host,
			Port: px.Port, Username: px.Username, HasPassword: px.Password != "" || len(px.PasswordCipher) > 0})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proxies": out})
}

// createProxy POST /admin/proxies — body: {name, scheme, host, port, username, password}
func (s *Server) createProxy(w http.ResponseWriter, r *http.Request) {
	var body proxyBody
	if !readBody(w, r, &body) || !body.validate() {
		http.Error(w, `{"error":"host and port required"}`, http.StatusBadRequest)
		return
	}
	sealed, err := account.EncryptCredential(s.accounts.DataDir(), []byte(body.Password))
	if err != nil {
		http.Error(w, `{"error":"password encryption failed"}`, http.StatusInternalServerError)
		return
	}
	rec := model.Proxy{Name: body.Name, Scheme: body.Scheme, Host: strings.TrimSpace(body.Host),
		Port: body.Port, Username: body.Username, PasswordCipher: sealed}
	if err := s.db.Create(&rec).Error; err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": rec.ID})
}

// updateProxy PUT /admin/proxies/{id} — body: {name, scheme, host, port, username, password}
// password 空字符串 = 保留原密码（前端不回显密码，避免误清空）。
func (s *Server) updateProxy(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var body proxyBody
	if !readBody(w, r, &body) || !body.validate() {
		http.Error(w, `{"error":"host and port required"}`, http.StatusBadRequest)
		return
	}
	fields := map[string]interface{}{
		"name": body.Name, "scheme": body.Scheme, "host": strings.TrimSpace(body.Host),
		"port": body.Port, "username": body.Username,
	}
	if body.Password != "" {
		sealed, err := account.EncryptCredential(s.accounts.DataDir(), []byte(body.Password))
		if err != nil {
			http.Error(w, `{"error":"password encryption failed"}`, http.StatusInternalServerError)
			return
		}
		fields["password_cipher"] = sealed
		fields["password"] = ""
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
	if id <= 0 {
		http.Error(w, `{"error":"invalid proxy id"}`, http.StatusBadRequest)
		return
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var px model.Proxy
		if err := tx.First(&px, id).Error; err != nil {
			return err
		}
		if err := tx.Where("proxy_id = ?", id).Delete(&model.GroupProxy{}).Error; err != nil {
			return err
		}
		if err := tx.Where("proxy_id = ?", id).Delete(&model.AccountProxy{}).Error; err != nil {
			return err
		}
		return tx.Delete(&px).Error
	})
	if err != nil {
		http.Error(w, `{"error":"proxy not deleted"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

var errInvalidProxyBinding = errors.New("invalid proxy binding")

// replaceProxyBindings 全部验证成功后事务替换，空集合表示显式解除绑定。
func (s *Server) replaceProxyBindings(scope string, id int64, ids []int64) error {
	if id <= 0 || ids == nil {
		return errInvalidProxyBinding
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		if scope == "account" {
			err = tx.First(&model.Account{}, id).Error
		} else {
			err = tx.First(&model.Group{}, id).Error
		}
		if err != nil {
			return err
		}
		unique := make(map[int64]bool)
		for _, pid := range ids {
			if pid <= 0 {
				return errInvalidProxyBinding
			}
			if err := tx.First(&model.Proxy{}, pid).Error; err != nil {
				return err
			}
			unique[pid] = true
		}
		if scope == "account" {
			if err := tx.Where("account_id = ?", id).Delete(&model.AccountProxy{}).Error; err != nil {
				return err
			}
			for pid := range unique {
				if err := tx.Create(&model.AccountProxy{AccountID: id, ProxyID: pid}).Error; err != nil {
					return err
				}
			}
		} else {
			if err := tx.Where("group_id = ?", id).Delete(&model.GroupProxy{}).Error; err != nil {
				return err
			}
			for pid := range unique {
				if err := tx.Create(&model.GroupProxy{GroupID: id, ProxyID: pid}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
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
	} else {
		var body proxyBody
		if !readBody(w, r, &body) || !body.validate() {
			http.Error(w, `{"error":"invalid proxy"}`, http.StatusBadRequest)
			return
		}
		px = model.Proxy{Scheme: body.Scheme, Host: body.Host, Port: body.Port, Username: body.Username, Password: body.Password}
	}
	config, err := account.ProxyConfig(s.accounts.DataDir(), &px)
	if err != nil {
		http.Error(w, `{"error":"proxy decryption failed"}`, http.StatusInternalServerError)
		return
	}
	px.Password = config.Password
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
	config := &pb.ProxyConfig{Scheme: px.Scheme, Host: strings.Trim(px.Host, "[]"), Port: px.Port, Username: px.Username, Password: px.Password}
	client := sdk.UpstreamClient(sdk.ProxyURL(config))
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected probe status %d", resp.StatusCode)
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
	if err := s.replaceProxyBindings("group", gid, body.ProxyIDs); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, errInvalidProxyBinding) || errors.Is(err, gorm.ErrRecordNotFound) {
			code = http.StatusBadRequest
		}
		http.Error(w, `{"error":"proxy bindings not saved"}`, code)
		return
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
	if err := s.replaceProxyBindings("account", aid, body.ProxyIDs); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, errInvalidProxyBinding) || errors.Is(err, gorm.ErrRecordNotFound) {
			code = http.StatusBadRequest
		}
		http.Error(w, `{"error":"proxy bindings not saved"}`, code)
		return
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
