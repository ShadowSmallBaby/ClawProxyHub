// Package model — GORM 实体定义。
// schema 变更只经 migrations/*.sql，这里仅做映射，禁止 AutoMigrate。
package model

import "time"

// User 管理员账号（系统设置模块）。
type User struct {
	ID           int64  `gorm:"primaryKey;autoIncrement"`
	Username     string `gorm:"uniqueIndex;size:64"`
	PasswordHash string `gorm:"size:128"` // bcrypt
	Role         string `gorm:"size:16;default:admin"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Plugin 已安装插件。
type Plugin struct {
	ID              int64  `gorm:"primaryKey;autoIncrement"`
	Name            string `gorm:"uniqueIndex;size:64"`
	Version         string `gorm:"size:32"`
	Author          string `gorm:"size:64"`
	ProtocolVersion int32
	ManifestJSON    string    `gorm:"column:manifest_json"`
	SettingsJSON    string    `gorm:"column:settings_json;default:'{}'"`
	Enabled         bool      `gorm:"default:true"`
	InstalledAt     time.Time `gorm:"column:installed_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

// TableName 显式声明，保持与迁移 SQL 的表名一致。
func (Plugin) TableName() string { return "plugins" }

// Account 账号（凭据 blob 由核心代管）。
type Account struct {
	ID             int64  `gorm:"primaryKey;autoIncrement"`
	PluginID       int64  `gorm:"index"`
	DisplayName    string `gorm:"size:128;default:''"`
	CredentialBlob []byte
	ProfileJSON    string `gorm:"column:profile_json;default:'{}'"`
	// 积分明细快照：{"total","used","remaining","packages":[...]}（插件解析上游后写入，读取只走库）
	CreditsJSON string `gorm:"column:credits_json;default:''"`
	// 账号模型目录快照：ModelInfo 数组 JSON，仅同步时写入，读取默认走库
	ModelsJSON string `gorm:"column:models_json;default:''"`
	Status     string `gorm:"size:16;default:active"` // active/disabled/expired
	// 自动暂停（429 限速 / 无积分等触发）：paused_until 到期自动恢复；reason 供展示
	PausedUntil   *time.Time `gorm:"column:paused_until"`
	PauseReason   string     `gorm:"column:pause_reason;size:256;default:''"`
	LastRefreshAt *time.Time `gorm:"column:last_refresh_at"`
	LastUsedAt    *time.Time `gorm:"column:last_used_at"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Account) TableName() string { return "accounts" }

// AccountGroup 账号↔分组多对多（仅同插件分组的组合有意义，由 API 校验）。
type AccountGroup struct {
	AccountID int64 `gorm:"primaryKey"`
	GroupID   int64 `gorm:"primaryKey"`
}

func (AccountGroup) TableName() string { return "account_groups" }

// Group 分组：某插件下的账号池（plugin_id 限定，跨插件无意义）。
type Group struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"uniqueIndex;size:64"`
	PluginID  int64  `gorm:"index"`
	Strategy  string `gorm:"size:32;default:round_robin"` // round_robin/random/least_used
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Key 对外密钥（加密存储，可回显）。授权路由为空 = 全部路由。
// KeyCipher：存量为 sha256 hex（仅校验用），新建为 AES-256-GCM 密文（0x01 前缀，可解密回显）。
type Key struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	KeyCipher string `gorm:"uniqueIndex;size:256;column:key_cipher"`
	Name      string `gorm:"size:128;default:''"`
	Enabled   bool   `gorm:"default:true"`
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// KeyRoute key ↔ 路由多对多授权。
type KeyRoute struct {
	KeyID   int64 `gorm:"primaryKey"`
	RouteID int64 `gorm:"primaryKey"`
}

func (KeyRoute) TableName() string { return "key_routes" }

// Route 路由：对外模型名 + 分组（含真实模型映射）权重表。
// 路由名即客户端请求的 model 字段。
type Route struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	Name       string `gorm:"uniqueIndex;size:128"`        // 对外模型名
	Strategy   string `gorm:"size:16;default:round_robin"` // round_robin/random/least_used/sticky
	GroupsJSON string `gorm:"column:groups_json;default:'[]'"`
	// 首事件超时（秒），0 = 跟随全局设置
	TimeoutSeconds int32 `gorm:"column:timeout_seconds;default:0"`
	// 降级：主分组失败且状态类匹配时切到 failover 分组的指定模型（每次请求至多降一次）
	FailoverEnabled bool   `gorm:"column:failover_enabled;default:false"`
	FailoverOn4xx   bool   `gorm:"column:failover_on_4xx;default:false"`
	FailoverOn5xx   bool   `gorm:"column:failover_on_5xx;default:false"`
	FailoverGroupID *int64 `gorm:"column:failover_group_id"`
	FailoverModel   string `gorm:"column:failover_model;size:128;default:''"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RouteGroupEntry groups_json 数组元素：分组 + 该分组下使用的真实模型 id。
type RouteGroupEntry struct {
	GroupID int64  `json:"group_id"`
	Weight  int    `json:"weight"`
	Model   string `json:"model"` // 分组对应插件的真实模型 id
}

// Proxy 出站代理。
type Proxy struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Name      string `gorm:"size:128;default:''"`
	Scheme    string `gorm:"size:16;default:http"`
	Host      string `gorm:"size:255"`
	Port      int32
	Username  string `gorm:"size:128;default:''"`
	Password  string `gorm:"size:128;default:''"`
	CreatedAt time.Time
}

// GroupProxy 分组与代理的多对多绑定。
type GroupProxy struct {
	GroupID int64 `gorm:"primaryKey"`
	ProxyID int64 `gorm:"primaryKey"`
}

func (GroupProxy) TableName() string { return "group_proxies" }

// AccountProxy 账号与代理的多对多绑定（优先级高于分组级）。
type AccountProxy struct {
	AccountID int64 `gorm:"primaryKey"`
	ProxyID   int64 `gorm:"primaryKey"`
}

func (AccountProxy) TableName() string { return "account_proxies" }

// TaskRule 调度规则：核心只描述"何时+对谁"。
type TaskRule struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	PluginID     int64
	CapabilityID string     `gorm:"column:capability_id;size:64"`
	TriggerType  string     `gorm:"column:trigger_type;size:16"` // interval/cron/daily/once
	TriggerValue string     `gorm:"column:trigger_value;size:128"`
	TargetScope  string     `gorm:"column:target_scope;size:16;default:all"` // all/rotate/account_ids
	TargetJSON   string     `gorm:"column:target_json;default:'[]'"`
	Enabled      bool       `gorm:"default:true"`
	LastRunAt    *time.Time `gorm:"column:last_run_at"`
	NextRunAt    *time.Time `gorm:"column:next_run_at;index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (TaskRule) TableName() string { return "task_rules" }

// TaskRun 任务执行历史。
type TaskRun struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	RuleID    *int64
	AccountID *int64
	Status    string `gorm:"size:16"` // running/success/failed
	Summary   string `gorm:"size:1024;default:''"`
	// 结构化明细快照（插件 RunTask 返回，如成长任务列表）
	DetailJSON   string     `gorm:"column:detail_json;default:''"`
	ErrorMessage string     `gorm:"column:error_message;size:1024;default:''"`
	StartedAt    time.Time  `gorm:"column:started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
}

func (TaskRun) TableName() string { return "task_runs" }

// RequestLog 调用日志。
type RequestLog struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	KeyID        *int64
	PluginID     *int64
	AccountID    *int64
	Model        string `gorm:"size:128;default:''"`         // 实际请求上游的真实模型
	RouteName    string `gorm:"column:route_name;size:128;default:''"` // 对外路由名（未命中路由为空）
	Protocol     string `gorm:"size:32;default:''"`
	Status       int32
	InputTokens  int32     `gorm:"column:input_tokens;default:0"`
	OutputTokens int32     `gorm:"column:output_tokens;default:0"`
	LatencyMs    int32     `gorm:"column:latency_ms;default:0"`
	FirstTokenMs int32     `gorm:"column:first_token_ms;default:0"` // 首字耗时
	CachedTokens int32     `gorm:"column:cached_tokens;default:0"`  // 缓存命中 token
	ClientIP     string    `gorm:"column:client_ip;size:64;default:''"`
	UserAgent    string    `gorm:"column:user_agent;size:256;default:''"`
	ErrorBrief   string    `gorm:"column:error_brief;size:512;default:''"`
	CreatedAt    time.Time `gorm:"index"`
}

func (RequestLog) TableName() string { return "request_logs" }

// Setting 系统设置 KV。
type Setting struct {
	Key       string `gorm:"primaryKey;size:128"`
	Value     string
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// OAuthCredential 第三方平台（LinuxDo/GitHub 等）用户登录态，供插件换取上游 token。
// TokenBlob 经核心 AES-256-GCM 加密存储（复用凭据加密密钥）。
type OAuthCredential struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	Platform     string     `gorm:"index;size:64"`
	AccountLabel string     `gorm:"column:account_label;size:128;default:''"`
	TokenBlob    []byte     `gorm:"column:token_blob"`
	ExpiresAt    *time.Time `gorm:"column:expires_at"`
	ExtraJSON    string     `gorm:"column:extra_json;default:'{}'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (OAuthCredential) TableName() string { return "oauth_credentials" }
