// accounts.go — 账号状态调度与详情。
package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
)

// updateAccount PUT /admin/accounts/{id} — body: {display_name?, group_ids?}
// group_ids 给定即全量替换账号分组（须同插件分组），空数组 = 移出全部分组。
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var body struct {
		DisplayName string  `json:"display_name"`
		GroupIDs    []int64 `json:"group_ids"`
		HasGroups   bool    `json:"-"`
	}
	if !readBody(w, r, &body) {
		return
	}
	var acct model.Account
	if err := s.db.First(&acct, id).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if body.DisplayName != "" {
		s.db.Model(&acct).Update("display_name", body.DisplayName)
	}
	if body.GroupIDs != nil {
		// 分组必须属于同一插件
		for _, gid := range body.GroupIDs {
			var g model.Group
			if err := s.db.First(&g, gid).Error; err != nil || g.PluginID != acct.PluginID {
				http.Error(w, `{"error":"分组不存在或与账号插件不一致"}`, http.StatusBadRequest)
				return
			}
		}
		s.db.Where("account_id = ?", id).Delete(&model.AccountGroup{})
		for _, gid := range body.GroupIDs {
			s.db.Create(&model.AccountGroup{AccountID: id, GroupID: gid})
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// accountGroupIDs 账号的全部分组 id。
func accountGroupIDs(db *gorm.DB, accountID int64) []int64 {
	var ids []int64
	db.Model(&model.AccountGroup{}).Where("account_id = ?", accountID).
		Order("group_id").Pluck("group_id", &ids)
	return ids
}

// pauseAccount POST /admin/accounts/{id}/pause — 手动停用调度（不参与选号，需手动恢复）。
func (s *Server) pauseAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	readBody(w, r, &body) // body 可省略
	s.db.Model(&model.Account{}).Where("id = ?", parseInt(r.PathValue("id"))).
		Updates(map[string]interface{}{
			"status":       "disabled",
			"pause_reason": truncStr(body.Reason, 250),
		})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// resumeAccount POST /admin/accounts/{id}/resume — 恢复调度（清自动暂停）。
// expired 账号恢复为 active 前提是凭据已重新可用，统一交由用户判断；此处一并置 active。
func (s *Server) resumeAccount(w http.ResponseWriter, r *http.Request) {
	s.db.Model(&model.Account{}).Where("id = ?", parseInt(r.PathValue("id"))).
		Updates(map[string]interface{}{
			"status": "active", "paused_until": nil, "pause_reason": "",
		})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// accountModels GET /admin/accounts/{id}/models — 同步客户端模型目录（账号凭据）。
func (s *Server) accountModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.accounts.Models(r.Context(), parseInt(r.PathValue("id")))
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
}

// accountDetail GET /admin/accounts/{id}/detail — 账号详情：基本信息 + 套餐/积分 + 任务执行情况。
func (s *Server) accountDetail(w http.ResponseWriter, r *http.Request) {
	var acct model.Account
	if err := s.db.First(&acct, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	var runs []model.TaskRun
	s.db.Where("account_id = ?", acct.ID).Order("started_at DESC").Limit(20).Find(&runs)
	var pauseUntil *string
	if acct.PausedUntil != nil {
		t := acct.PausedUntil.Format("2006-01-02 15:04:05")
		pauseUntil = &t
	}
	manualPause := acct.PausedUntil != nil && acct.PausedUntil.After(time.Now().AddDate(50, 0, 0))

	out := map[string]interface{}{
		"id":              acct.ID,
		"plugin_id":       acct.PluginID,
		"group_ids":       accountGroupIDs(s.db, acct.ID),
		"display_name":    acct.DisplayName,
		"status":          acct.Status,
		"pause_reason":    acct.PauseReason,
		"paused_until":    pauseUntil,
		"manual_pause":    manualPause, // true = 需手动恢复（402 无积分等）
		"last_refresh_at": acct.LastRefreshAt,
		"last_used_at":    acct.LastUsedAt,
		"created_at":      acct.CreatedAt,
		"profile":         jsonOrNull(acct.ProfileJSON),
		"credits":         jsonOrNull(acct.CreditsJSON),
		"runs":            s.runViews(runs),
	}
	// 套餐/积分信息在 profile 快照里（插件 GetProfile / 登录返回）
	writeJSON(w, http.StatusOK, out)
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// jsonOrNull 原样透出存储的 JSON 快照（异常时回空对象）。
func jsonOrNull(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s)
}
