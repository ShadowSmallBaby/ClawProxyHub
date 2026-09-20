// oauth.go — 第三方平台（LinuxDo/GitHub 等）用户登录态托管。
// 供插件换取上游 token；token 经核心 AES-256-GCM 加密存储，列表不回显明文。
package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// oauthView OAuth 凭据列表项（不含 token 明文，只标记是否已设置）。
type oauthView struct {
	ID           int64   `json:"id"`
	Platform     string  `json:"platform"`
	AccountLabel string  `json:"account_label"`
	HasToken     bool    `json:"has_token"`
	ExpiresAt    *string `json:"expires_at"`
	ExtraJSON    string  `json:"extra_json"`
	CreatedAt    string  `json:"created_at"`
}

// listOAuth GET /admin/oauth-credentials — 平台登录态列表（token 不回显）。
func (s *Server) listOAuth(w http.ResponseWriter, r *http.Request) {
	var creds []model.OAuthCredential
	if err := s.db.Order("id").Find(&creds).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	out := make([]oauthView, 0, len(creds))
	for _, c := range creds {
		v := oauthView{
			ID: c.ID, Platform: c.Platform, AccountLabel: c.AccountLabel,
			HasToken: len(c.TokenBlob) > 0, ExtraJSON: c.ExtraJSON,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		}
		if c.ExpiresAt != nil {
			t := c.ExpiresAt.Format(time.RFC3339)
			v.ExpiresAt = &t
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"credentials": out})
}

// oauthBody 新建/编辑请求体。
type oauthBody struct {
	Platform     string `json:"platform"`
	AccountLabel string `json:"account_label"`
	Token        string `json:"token"`      // 明文登录态，落库前加密；编辑留空 = 保留原值
	ExpiresAt    string `json:"expires_at"` // RFC3339，可空
	ExtraJSON    string `json:"extra_json"`
}

// parseExpires 解析可选过期时间。
func parseExpires(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

// createOAuth POST /admin/oauth-credentials — 新增平台登录态。
func (s *Server) createOAuth(w http.ResponseWriter, r *http.Request) {
	var body oauthBody
	if !readBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Platform) == "" {
		http.Error(w, `{"error":"platform required"}`, http.StatusBadRequest)
		return
	}
	extra := body.ExtraJSON
	if strings.TrimSpace(extra) == "" {
		extra = "{}"
	}
	rec := model.OAuthCredential{
		Platform: body.Platform, AccountLabel: body.AccountLabel,
		TokenBlob: account.EncryptCredential(s.accounts.DataDir(), []byte(body.Token)),
		ExpiresAt: parseExpires(body.ExpiresAt), ExtraJSON: extra,
	}
	if err := s.db.Create(&rec).Error; err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": rec.ID})
}

// updateOAuth PUT /admin/oauth-credentials/{id} — 编辑；token 留空保留原值。
func (s *Server) updateOAuth(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var rec model.OAuthCredential
	if err := s.db.First(&rec, id).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	var body oauthBody
	if !readBody(w, r, &body) {
		return
	}
	fields := map[string]interface{}{
		"account_label": body.AccountLabel,
		"expires_at":    parseExpires(body.ExpiresAt),
	}
	if strings.TrimSpace(body.Platform) != "" {
		fields["platform"] = body.Platform
	}
	if strings.TrimSpace(body.ExtraJSON) != "" {
		fields["extra_json"] = body.ExtraJSON
	}
	// token 留空 = 保留原值（不回显，编辑时无需重填）
	if body.Token != "" {
		fields["token_blob"] = account.EncryptCredential(s.accounts.DataDir(), []byte(body.Token))
	}
	s.db.Model(&rec).Updates(fields)
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// deleteOAuth DELETE /admin/oauth-credentials/{id}
func (s *Server) deleteOAuth(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Delete(&model.OAuthCredential{}, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
