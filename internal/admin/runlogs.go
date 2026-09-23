// runlogs.go — 运行日志查询 API（列表 / 清空；写入在各业务模块埋点）。
package admin

import (
	"net/http"
	"strings"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// routeRunLogs 运行日志端点（注册于 routeLogs）。
func (s *Server) routeRunLogs(r authed) {
	r.h("GET /admin/run-logs", s.listRunLogs)
	r.h("DELETE /admin/run-logs", s.clearRunLogs)
}

// listRunLogs GET /admin/run-logs?page=&page_size=&level=&module=&from=&to=
func (s *Server) listRunLogs(w http.ResponseWriter, r *http.Request) {
	qp := r.URL.Query()
	// 分页：page 从 1 起，page_size 限定档位（默认 30）
	pageSize := 30
	switch parseInt(qp.Get("page_size")) {
	case 10, 30, 50, 100, 200:
		pageSize = int(parseInt(qp.Get("page_size")))
	}
	page := int(parseInt(qp.Get("page")))
	if page < 1 {
		page = 1
	}
	q := s.db.Model(&model.RunLog{})
	if lv := strings.TrimSpace(qp.Get("level")); lv != "" {
		q = q.Where("level = ?", lv)
	}
	if md := strings.TrimSpace(qp.Get("module")); md != "" {
		q = q.Where("module = ?", md)
	}
	if kw := strings.TrimSpace(qp.Get("keyword")); kw != "" {
		kw = "%" + kw + "%"
		q = q.Where("message LIKE ? OR action LIKE ? OR detail LIKE ?", kw, kw, kw)
	}
	if from := strings.TrimSpace(qp.Get("from")); from != "" {
		q = q.Where("created_at >= ?", from)
	}
	if to := strings.TrimSpace(qp.Get("to")); to != "" {
		q = q.Where("created_at <= ?", to)
	}
	var total int64
	q.Count(&total)
	var logs []model.RunLog
	if err := q.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&logs).Error; err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"logs": logs, "total": total})
}

// clearRunLogs DELETE /admin/run-logs — 清空全部运行日志，返回删除条数。
func (s *Server) clearRunLogs(w http.ResponseWriter, r *http.Request) {
	res := s.db.Exec("DELETE FROM run_logs")
	if res.Error != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": res.RowsAffected})
}
