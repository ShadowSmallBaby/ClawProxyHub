// Package account — 账号服务：登录多步协议、建档、刷新、过期处理。
package account

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/event"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/plugin"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// ErrUnauthorized 通用业务错误。
type ErrUnauthorized string

func (e ErrUnauthorized) Error() string { return string(e) }

// IsAuthFailure 终态认证失败（凭据不可自愈）。
func IsAuthFailure(err error) bool {
	_, ok := err.(ErrUnauthorized)
	return ok
}

// Service 账号域服务。
type Service struct {
	db      *gorm.DB
	dataDir string
	mgr     *plugin.Manager
}

func New(db *gorm.DB, dataDir string, mgr *plugin.Manager) *Service {
	return &Service{db: db, dataDir: dataDir, mgr: mgr}
}

// DataDir 数据目录（解密密钥等用途）。
func (s *Service) DataDir() string { return s.dataDir }

// AuthMethods 插件声明的授权方式（前端渲染 tab + 表单）。
func (s *Service) AuthMethods(pluginName string) ([]*pb.AuthMethod, error) {
	inst, ok := s.mgr.Get(pluginName)
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}
	return inst.Manifest.GetAuthMethods(), nil
}

// SubmitLogin 提交一步登录。完成时自动建档并返回账号 id；未完成返回下一步。
// instanceID 为目标实例（0 = 插件默认实例）；调用方（管理 API）只需把 next 原样透给前端。
func (s *Service) SubmitLogin(ctx context.Context, pluginName, methodID string, form map[string]string, state []byte, instanceID int64) (*LoginOutcome, error) {
	inst, ok := s.mgr.Get(pluginName)
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}
	var p model.Plugin
	if err := s.db.Where("name = ?", pluginName).First(&p).Error; err != nil {
		return nil, fmt.Errorf("plugin record missing for %q", pluginName)
	}
	target, err := ResolveInstance(s.db, p.ID, instanceID, inst.MultiInstance())
	if err != nil {
		return nil, err
	}

	result, err := inst.Client().Login(ctx, &pb.LoginRequest{
		MethodId: methodID, Form: form, State: state, InstanceId: target.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin login: %w", err)
	}
	if result.Error != nil && result.Error.Code != 0 {
		return nil, ErrUnauthorized(result.Error.Message)
	}
	if result.Next != nil {
		return &LoginOutcome{Next: result.Next}, nil
	}

	// 登录完成 → 建档
	acct, err := s.create(p.ID, target.ID, result.Blob, result.Profile)
	if err != nil {
		return nil, err
	}
	// 首次建档自动拉一次模型目录落库（best-effort，失败不阻断登录）
	_, _ = s.SyncModels(ctx, acct.ID)
	return &LoginOutcome{AccountID: acct.ID, Profile: result.Profile}, nil
}

// LoginOutcome 登录一步的结果：建档完成 或 需要下一步。
type LoginOutcome struct {
	AccountID int64             // >0 表示建档完成
	Next      *pb.LoginNextStep // 非 nil 表示登录未完成
	Profile   *pb.AccountProfile
}

// create 凭据入库（账号归属 pluginID/instanceID 已由调用方校验）。
// profile.healthy=false：鉴权有效但暂不可用于对话调度（如尚未取得 API 密钥）→ 以 disabled 入库，任务照常。
func (s *Service) create(pluginID, instanceID int64, blob []byte, profile *pb.AccountProfile) (*model.Account, error) {
	name := ""
	profileJSON := "{}"
	creditsJSON := ""
	status, reason := "active", ""
	if profile != nil {
		name = profile.DisplayName
		// 登录返回的 profile（含 quota）直接入库，积分首刷前即有值
		if b, err := protojson.Marshal(profile); err == nil {
			profileJSON = string(b)
		}
		creditsJSON = profile.CreditsJson
		if !profile.Healthy {
			status, reason = "disabled", unhealthyReason
		}
	}
	acct := &model.Account{
		PluginID: pluginID, InstanceID: instanceID, DisplayName: name,
		CredentialBlob: EncryptCredential(s.dataDir, blob), Status: status, PauseReason: reason,
		ProfileJSON: profileJSON, CreditsJSON: creditsJSON, LastRefreshAt: ptrTime(time.Now()),
	}
	if err := s.db.Create(acct).Error; err != nil {
		return nil, err
	}
	return acct, nil
}

// unhealthyReason 插件报告 healthy=false 时的停用原因（前端调度列展示）。
const unhealthyReason = "凭据暂不可用于调度（如未取得 API 密钥），刷新取得后可手动启用"

// Schedulable 账号凭据是否可用于对话调度（profile.healthy；无快照视为可用）。
func Schedulable(acct *model.Account) bool {
	if acct.ProfileJSON == "" {
		return true
	}
	var p pb.AccountProfile
	if protojson.Unmarshal([]byte(acct.ProfileJSON), &p) != nil {
		return true
	}
	return p.Healthy
}

// Refresh 刷新单账号凭据。刷新失败且凭据确已失效时标记 expired。
func (s *Service) Refresh(ctx context.Context, accountID int64) (*model.Account, error) {
	var acct model.Account
	if err := s.db.First(&acct, accountID).Error; err != nil {
		return nil, err
	}
	pluginName, err := s.pluginName(acct.PluginID)
	if err != nil {
		return nil, err
	}
	inst, ok := s.mgr.Get(pluginName)
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}

	cred := BuildCred(s.db, s.dataDir, &acct, 0)
	result, err := inst.Client().Refresh(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("plugin refresh: %w", err)
	}
	if result.Error != nil && result.Error.Code != 0 {
		// 401 类错误：凭据失效，标记过期（换号重试会跳过）
		if result.Error.Code == 401 {
			s.db.Model(&acct).Update("status", "expired")
		}
		return nil, ErrUnauthorized(result.Error.Message)
	}

	updates := map[string]interface{}{"last_refresh_at": time.Now()}
	if len(result.Blob) > 0 {
		updates["credential_blob"] = EncryptCredential(s.dataDir, result.Blob)
	}
	if result.Profile != nil {
		updates["profile_json"] = profileJSON(result.Profile)
		if result.Profile.DisplayName != "" {
			updates["display_name"] = result.Profile.DisplayName
		}
		// 积分明细快照：插件解析了才更新，为空保留旧值（避免无明细的插件抹掉已有数据）
		if result.Profile.CreditsJson != "" {
			updates["credits_json"] = result.Profile.CreditsJson
		}
	}
	// 状态：expired → active（凭据已恢复）；disabled 保留（用户手动停用 / 尚不可调度，取得凭据后手动启用）；
	// 插件报告 healthy=false → disabled 并标注原因
	switch {
	case result.Profile != nil && !result.Profile.Healthy:
		updates["status"], updates["pause_reason"] = "disabled", unhealthyReason
	case acct.Status == "expired":
		updates["status"] = "active"
	case acct.Status == "disabled" && acct.PauseReason == unhealthyReason:
		updates["pause_reason"] = "" // 凭据已可用，等待手动启用
	}
	s.db.Model(&acct).Updates(updates)
	// 插件要求提醒（如密钥已更换需重新同步模型）→ 站内通知
	if n := result.Notification; n != nil && n.Title != "" {
		s.db.Create(&model.Notification{
			Title: truncStr(n.Title, 256), Content: truncStr(n.Content, 4000),
			Level: orDefault(n.Level, "info"), AccountID: &acct.ID,
		})
	}
	s.db.First(&acct, accountID)
	return &acct, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Models 用账号凭据拉插件模型目录（账号级可见模型）。
func (s *Service) Models(ctx context.Context, accountID int64) ([]*pb.ModelInfo, error) {
	var acct model.Account
	if err := s.db.First(&acct, accountID).Error; err != nil {
		return nil, err
	}
	pluginName, err := s.pluginName(acct.PluginID)
	if err != nil {
		return nil, err
	}
	cred := BuildCred(s.db, s.dataDir, &acct, 0)
	inst, ok := s.mgr.Get(pluginName)
	if !ok {
		return nil, fmt.Errorf("plugin %q not running", pluginName)
	}
	ml, err := inst.Client().ListModels(ctx, cred)
	if err != nil {
		return nil, err
	}
	return ml.Models, nil
}

// SyncModels 拉上游模型目录并落库（首次建档 / 手动刷新用），返回拉取结果。
func (s *Service) SyncModels(ctx context.Context, accountID int64) ([]*pb.ModelInfo, error) {
	models, err := s.Models(ctx, accountID)
	if err != nil {
		return nil, err
	}
	s.db.Model(&model.Account{}).Where("id = ?", accountID).
		Update("models_json", marshalModels(models))
	return models, nil
}

// SaveModels 存用户勾选的模型目录（以用户为准）。
func (s *Service) SaveModels(accountID int64, models []*pb.ModelInfo) {
	s.db.Model(&model.Account{}).Where("id = ?", accountID).
		Update("models_json", marshalModels(models))
}

// StoredModels 读库中的模型目录快照。
func (s *Service) StoredModels(accountID int64) []*pb.ModelInfo {
	var acct model.Account
	if err := s.db.Select("models_json").First(&acct, accountID).Error; err != nil || acct.ModelsJSON == "" {
		return nil
	}
	var raws []json.RawMessage
	if json.Unmarshal([]byte(acct.ModelsJSON), &raws) != nil {
		return nil
	}
	out := make([]*pb.ModelInfo, 0, len(raws))
	for _, r := range raws {
		m := &pb.ModelInfo{}
		if protojson.Unmarshal(r, m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// marshalModels ModelInfo 数组 → JSON（protojson 保真，逐条编码）。
func marshalModels(models []*pb.ModelInfo) string {
	raws := make([]json.RawMessage, 0, len(models))
	for _, m := range models {
		if b, err := protojson.Marshal(m); err == nil {
			raws = append(raws, b)
		}
	}
	b, err := json.Marshal(raws)
	if err != nil {
		return ""
	}
	return string(b)
}

// MarkExpired 标记账号过期（网关 401 换号路径调用）。
func (s *Service) MarkExpired(accountID int64) {
	s.db.Model(&model.Account{}).Where("id = ?", accountID).
		Update("status", "expired")
}

// MarkAutoPause 自动暂停选号（429 限速 / 402 无积分等触发）。
// resumeAt 到期自动恢复；传 nil 写入远期时间 = 需手动恢复。仅对 active 账号生效。
func (s *Service) MarkAutoPause(accountID int64, reason string, resumeAt *time.Time) {
	if resumeAt == nil {
		far := time.Now().AddDate(100, 0, 0) // 远期哨兵：选号条件统一按 paused_until 判断
		resumeAt = &far
	}
	s.db.Model(&model.Account{}).Where("id = ? AND status = ?", accountID, "active").
		Updates(map[string]interface{}{
			"paused_until": resumeAt,
			"pause_reason": truncStr(reason, 250),
		})
}

// Resume 清除自动暂停（手动恢复入口；不影响 status，expired 仍需重新授权）。
func (s *Service) Resume(accountID int64) {
	s.db.Model(&model.Account{}).Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"paused_until": nil, "pause_reason": "",
		})
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// List 插件维度的账号列表（凭据不外泄）。
func (s *Service) List(pluginID int64) ([]model.Account, error) {
	var accts []model.Account
	err := s.db.Where("plugin_id = ?", pluginID).Order("id").Find(&accts).Error
	return accts, err
}

// Delete 删除账号（凭据随之清除）。
func (s *Service) pluginName(pluginID int64) (string, error) {
	var p model.Plugin
	if err := s.db.First(&p, pluginID).Error; err != nil {
		return "", fmt.Errorf("plugin record #%d missing", pluginID)
	}
	return p.Name, nil
}

// profileJSON AccountProfile 序列化为快照存储。
func profileJSON(p *pb.AccountProfile) string {
	b, err := protojson.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func ptrTime(t time.Time) *time.Time { return &t }

// SubscribeRefresh 订阅任务完成事件，成功后刷新该账号 profile（积分/套餐）。
func (s *Service) SubscribeRefresh(ctx context.Context, bus *event.Bus) {
	ch := bus.Subscribe(event.TopicTaskCompleted)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				if ev.AccountID > 0 {
					_, _ = s.Refresh(ctx, ev.AccountID)
				}
			}
		}
	}()
}
