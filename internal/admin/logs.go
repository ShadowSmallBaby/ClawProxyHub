// logs.go — 调用日志维护：清空 / CSV 导出（筛选条件与列表共用 logsQuery）。
package admin

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// routeLogs 日志维护端点。
func (s *Server) routeLogs(r authed) {
	r.h("GET /admin/logs", s.listLogs)
	r.h("GET /admin/logs/export", s.exportLogs)
	r.h("DELETE /admin/logs", s.clearLogs)
}

// clearLogs DELETE /admin/logs — 清空全部调用日志（不带筛选），返回删除条数。
func (s *Server) clearLogs(w http.ResponseWriter, r *http.Request) {
	res := s.db.Exec("DELETE FROM request_logs")
	if res.Error != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": res.RowsAffected})
}

// exportLogs GET /admin/logs/export?<同列表筛选> — CSV 流式导出（UTF-8 BOM，Excel 可直接打开）；限 admin。
func (s *Server) exportLogs(w http.ResponseWriter, r *http.Request) {
	if roleOf(r) != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="cph-logs-%s.csv"`, time.Now().Format("20060102-150405")))
	w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "time", "key_id", "plugin_id", "account_id", "route", "model", "protocol", "status",
		"input_tokens", "output_tokens", "cached_tokens", "latency_ms", "first_token_ms", "client_ip", "user_agent", "error"})

	// 按 id 游标分批读，避免一次性加载全表
	var lastID int64
	for {
		var batch []model.RequestLog
		if err := s.logsQuery(r.URL.Query()).Where("id > ?", lastID).Order("id").Limit(2000).Find(&batch).Error; err != nil || len(batch) == 0 {
			break
		}
		for _, l := range batch {
			cw.Write([]string{
				strconv.FormatInt(l.ID, 10), l.CreatedAt.Format("2006-01-02 15:04:05"),
				ptrInt(l.KeyID), ptrInt(l.PluginID), ptrInt(l.AccountID),
				l.RouteName, l.Model, l.Protocol, strconv.Itoa(int(l.Status)),
				strconv.Itoa(int(l.InputTokens)), strconv.Itoa(int(l.OutputTokens)), strconv.Itoa(int(l.CachedTokens)),
				strconv.Itoa(int(l.LatencyMs)), strconv.Itoa(int(l.FirstTokenMs)),
				l.ClientIP, l.UserAgent, l.ErrorBrief,
			})
			lastID = l.ID
		}
		cw.Flush()
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	cw.Flush()
}

func ptrInt(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}
