// notifications.go — 站内通知（任务产生的提醒）：列表 / 已读 / 全部已读 / 清理已读。
package admin

import (
	"net/http"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// routeNotifications 通知端点（guest 可读列表；写操作由 auth 中间件按角色拦截）。
func (s *Server) routeNotifications(r authed) {
	r.h("GET /admin/notifications", s.listNotifications)
	r.h("POST /admin/notifications/{id}/read", s.readNotification)
	r.h("POST /admin/notifications/read-all", s.readAllNotifications)
	r.h("DELETE /admin/notifications", s.clearReadNotifications)
}

// listNotifications GET /admin/notifications?limit=50 — 最新在前 + 未读数。
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if n := parseInt(r.URL.Query().Get("limit")); n > 0 && n <= 200 {
		limit = int(n)
	}
	var list []model.Notification
	s.db.Order("id DESC").Limit(limit).Find(&list)
	var unread int64
	s.db.Model(&model.Notification{}).Where("read = ?", false).Count(&unread)
	type view struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		Level     string `json:"level"`
		Read      bool   `json:"read"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]view, 0, len(list))
	for _, n := range list {
		out = append(out, view{ID: n.ID, Title: n.Title, Content: n.Content, Level: n.Level, Read: n.Read,
			CreatedAt: n.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"notifications": out, "unread": unread})
}

// readNotification POST /admin/notifications/{id}/read
func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) {
	s.db.Model(&model.Notification{}).Where("id = ?", parseInt(r.PathValue("id"))).Update("read", true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// readAllNotifications POST /admin/notifications/read-all
func (s *Server) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	s.db.Model(&model.Notification{}).Where("read = ?", false).Update("read", true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// clearReadNotifications DELETE /admin/notifications — 删除全部已读通知。
func (s *Server) clearReadNotifications(w http.ResponseWriter, r *http.Request) {
	res := s.db.Where("read = ?", true).Delete(&model.Notification{})
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": res.RowsAffected})
}
