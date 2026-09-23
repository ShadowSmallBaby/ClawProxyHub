// accounts.go — 账号状态调度与详情。
package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/account"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// updateAccount PUT /admin/accounts/{id} — body: {display_name?, group_ids?, instance_id?}
// group_ids 给定即全量替换账号分组（须同插件分组），空数组 = 移出全部分组；
// instance_id>0 改归属实例（须同插件实例）。
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	var body struct {
		DisplayName string  `json:"display_name"`
		GroupIDs    []int64 `json:"group_ids"`
		InstanceID  int64   `json:"instance_id"`
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
	if body.InstanceID > 0 && body.InstanceID != acct.InstanceID {
		inst, err := account.ResolveInstance(s.db, acct.PluginID, body.InstanceID, s.multiInstance(acct.PluginID))
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		// 分组归属实例：换实例前须先移出旧实例的分组（本次请求给了 group_ids 时以其为准校验）
		if body.GroupIDs == nil && len(accountGroupIDs(s.db, acct.ID)) > 0 {
			http.Error(w, `{"error":"账号仍在旧实例的分组中，请先移出分组再更换实例"}`, http.StatusBadRequest)
			return
		}
		s.db.Model(&acct).Update("instance_id", inst.ID)
		acct.InstanceID = inst.ID
	}
	if body.GroupIDs != nil {
		// 分组必须属于同一插件且同一实例
		for _, gid := range body.GroupIDs {
			var g model.Group
			if err := s.db.First(&g, gid).Error; err != nil || g.PluginID != acct.PluginID || g.InstanceID != acct.InstanceID {
				http.Error(w, `{"error":"分组不存在或与账号插件/实例不一致"}`, http.StatusBadRequest)
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
// 插件仍报告 healthy=false（如尚未取得 API 密钥）时拒绝，需先刷新。
func (s *Server) resumeAccount(w http.ResponseWriter, r *http.Request) {
	var acct model.Account
	if err := s.db.First(&acct, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if !account.Schedulable(&acct) {
		http.Error(w, `{"error":"凭据暂不可用于调度（如尚未取得 API 密钥），请先刷新账号"}`, http.StatusConflict)
		return
	}
	s.db.Model(&acct).
		Updates(map[string]interface{}{
			"status": "active", "paused_until": nil, "pause_reason": "",
		})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// accountModels GET /admin/accounts/{id}/models — 默认读库快照；?refresh=1 才实时拉上游并落库。
func (s *Server) accountModels(w http.ResponseWriter, r *http.Request) {
	id := parseInt(r.PathValue("id"))
	if r.URL.Query().Get("refresh") == "1" {
		models, err := s.accounts.SyncModels(r.Context(), id)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"models": s.accounts.StoredModels(id)})
}

// saveAccountModels PUT /admin/accounts/{id}/models — 存用户勾选的模型目录（以用户为准）。
func (s *Server) saveAccountModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Models []json.RawMessage `json:"models"`
	}
	if !readBody(w, r, &body) {
		return
	}
	models := make([]*pb.ModelInfo, 0, len(body.Models))
	for _, raw := range body.Models {
		m := &pb.ModelInfo{}
		if protojson.Unmarshal(raw, m) == nil && m.Id != "" {
			models = append(models, m)
		}
	}
	s.accounts.SaveModels(parseInt(r.PathValue("id")), models)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// testAccount POST /admin/accounts/{id}/test — body: {endpoint, model, question?}
// 用账号凭据直调插件 Chat，绕过路由/key，收集事件为日志返回，不落 request_logs。
func (s *Server) testAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Model    string `json:"model"`
		Question string `json:"question"`
	}
	if !readBody(w, r, &body) {
		return
	}
	var acct model.Account
	if err := s.db.First(&acct, parseInt(r.PathValue("id"))).Error; err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	pluginName := pluginNameByID(s.db, acct.PluginID)
	if pluginName == "" {
		http.Error(w, `{"error":"plugin not found"}`, http.StatusBadRequest)
		return
	}
	question := body.Question
	if question == "" {
		question = "你好，请用一句话自我介绍。"
	}
	req := &pb.ChatRequest{
		Model:  body.Model,
		Source: body.Endpoint,
		Messages: []*pb.EnvelopeMessage{
			{Role: "user", Text: question},
		},
	}
	cred := account.BuildCred(s.db, s.accounts.DataDir(), &acct, 0)

	// 信封请求（诊断展示用）：清掉凭据再序列化
	req.Credential = nil
	reqJSON, _ := protojson.Marshal(req)
	req.Credential = cred

	events, err := s.plugins.Chat(req, pluginName, cred)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	var text string
	var logs []string
	var evLines []string
	for ev := range events {
		// 事件明细（诊断用，上限 200 防爆）
		if len(evLines) < 200 {
			if raw, err := protojson.Marshal(ev); err == nil {
				evLines = append(evLines, string(raw))
			}
		}
		switch e := ev.Event.(type) {
		case *pb.StreamEvent_MessageStart:
			logs = append(logs, "→ model: "+e.MessageStart.Model)
		case *pb.StreamEvent_ContentDelta:
			text += e.ContentDelta.Text
		case *pb.StreamEvent_ToolCallDelta:
			logs = append(logs, "→ tool_call: "+e.ToolCallDelta.Name+" "+e.ToolCallDelta.ArgumentsDelta)
		case *pb.StreamEvent_MessageFinish:
			if e.MessageFinish.Usage != nil {
				logs = append(logs, fmt.Sprintf("→ finish: %s (in=%d out=%d)",
					e.MessageFinish.FinishReason, e.MessageFinish.Usage.InputTokens, e.MessageFinish.Usage.OutputTokens))
			} else {
				logs = append(logs, "→ finish: "+e.MessageFinish.FinishReason)
			}
		case *pb.StreamEvent_TaskFailed:
			logs = append(logs, fmt.Sprintf("✗ failed: code=%d %s", e.TaskFailed.Error.GetCode(), e.TaskFailed.Error.GetMessage()))
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"text": text, "logs": logs, "request": string(reqJSON), "events": evLines,
	})
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
		"instance_id":     acct.InstanceID,
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
		"models":          s.accounts.StoredModels(acct.ID),
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
