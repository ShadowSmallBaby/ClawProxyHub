-- 000001_init.up.sql — ClawProxyHub 初始 schema（SQLite 方言）
-- 核心是唯一写 schema 的角色；插件凭据/状态经 ClawHost RPC 流转，不建表。

-- 管理员（系统设置模块持有，首次启动 seed）
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,               -- bcrypt
    role          TEXT    NOT NULL DEFAULT 'admin',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 已安装插件
CREATE TABLE IF NOT EXISTS plugins (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT    NOT NULL UNIQUE,     -- 对应 manifest.name
    version          TEXT    NOT NULL,
    author           TEXT    NOT NULL,
    protocol_version INTEGER NOT NULL,
    manifest_json    TEXT    NOT NULL,            -- 完整 manifest 快照
    settings_json    TEXT    NOT NULL DEFAULT '{}', -- 插件设置（settings_schema 校验后的值）
    enabled          INTEGER NOT NULL DEFAULT 1,  -- 持久化启停：停止写 0，重启核心保持停止；启动/重启插件写回 1
    installed_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 账号（凭据 blob 由核心代管；插件经 RPC 拿到解密后的内容）
CREATE TABLE IF NOT EXISTS accounts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id       INTEGER NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    display_name    TEXT    NOT NULL DEFAULT '',
    credential_blob BLOB,                        -- 插件自定义格式，核心不解析
    profile_json    TEXT    NOT NULL DEFAULT '{}', -- AccountProfile 快照（额度/健康）
    credits_json    TEXT    NOT NULL DEFAULT '',  -- 积分明细快照（插件解析上游后写入，读取只走库）
    status          TEXT    NOT NULL DEFAULT 'active', -- active/disabled/expired
    paused_until    DATETIME,                    -- 自动暂停：到期自动恢复（429 限速等）；NULL/未到期 = 不参与选号
    pause_reason    TEXT    NOT NULL DEFAULT '',
    last_refresh_at DATETIME,
    last_used_at    DATETIME,                    -- least_used 排序依据 + 粘性过期判断
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_accounts_plugin ON accounts(plugin_id);

-- 分组：某插件下的账号池（plugin_id 限定，跨插件无意义）
CREATE TABLE IF NOT EXISTS groups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE,
    plugin_id  INTEGER REFERENCES plugins(id) ON DELETE CASCADE,
    strategy   TEXT    NOT NULL DEFAULT 'round_robin', -- round_robin/random/least_used
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_groups_plugin ON groups(plugin_id);

-- 账号↔分组多对多（一个账号可入多个同插件分组）
CREATE TABLE IF NOT EXISTS account_groups (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    group_id   INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (account_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_account_groups_group ON account_groups(group_id);

-- 路由：对外模型别名 → 分组（含真实模型映射）权重表
-- 账号选择策略：round_robin / random / least_used / sticky
CREATE TABLE IF NOT EXISTS routes (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT    NOT NULL UNIQUE,     -- 对外模型名，即客户端请求的 model 字段
    strategy          TEXT    NOT NULL DEFAULT 'round_robin',
    groups_json       TEXT    NOT NULL DEFAULT '[]', -- [{"group_id":1,"weight":100,"model":"..."}]
    timeout_seconds   INTEGER NOT NULL DEFAULT 0,  -- 首事件超时，0 = 跟随全局设置
    failover_enabled  INTEGER NOT NULL DEFAULT 0,  -- 降级：主分组失败时切到 failover 分组（每次请求至多降一次）
    failover_on_4xx   INTEGER NOT NULL DEFAULT 0,  -- 上游 4xx 触发降级
    failover_on_5xx   INTEGER NOT NULL DEFAULT 0,  -- 上游 5xx / 超时触发降级
    failover_group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL,
    failover_model    TEXT    NOT NULL DEFAULT '',
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 对外密钥（AES-256-GCM 加密存储，可回显；复用凭据加密密钥）
CREATE TABLE IF NOT EXISTS keys (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    key_cipher TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 密钥↔路由多对多授权（空 = 全部路由）
CREATE TABLE IF NOT EXISTS key_routes (
    key_id   INTEGER NOT NULL REFERENCES keys(id) ON DELETE CASCADE,
    route_id INTEGER NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    PRIMARY KEY (key_id, route_id)
);

-- 出站代理（按分组关联）
CREATE TABLE IF NOT EXISTS proxies (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL DEFAULT '',
    scheme     TEXT    NOT NULL DEFAULT 'http',
    host       TEXT    NOT NULL,
    port       INTEGER NOT NULL,
    username   TEXT    NOT NULL DEFAULT '',
    password   TEXT    NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS group_proxies (
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    proxy_id INTEGER NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, proxy_id)
);

-- 调度规则（核心只存"何时+对谁"，能力语义在插件）
CREATE TABLE IF NOT EXISTS task_rules (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id     INTEGER NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    capability_id TEXT    NOT NULL,              -- 对应 TaskCapability.id
    trigger_type  TEXT    NOT NULL,              -- interval / cron / daily / once
    trigger_value TEXT    NOT NULL,              -- 如 "3600s" / "0 9 * * *" / "09:00" / RFC3339
    target_scope  TEXT    NOT NULL DEFAULT 'all', -- all / rotate / account_ids
    target_json   TEXT    NOT NULL DEFAULT '[]', -- account_ids 列表
    enabled       INTEGER NOT NULL DEFAULT 1,
    last_run_at   DATETIME,
    next_run_at   DATETIME,                     -- 引擎计算的下次触发时刻
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_task_rules_next ON task_rules(enabled, next_run_at);

-- 任务执行历史
CREATE TABLE IF NOT EXISTS task_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id       INTEGER REFERENCES task_rules(id) ON DELETE CASCADE,
    account_id    INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    status        TEXT    NOT NULL,              -- running/success/failed
    summary       TEXT    NOT NULL DEFAULT '',
    detail_json   TEXT    NOT NULL DEFAULT '',   -- 结构化明细快照（如成长任务列表，账号详情直接渲染）
    error_message TEXT    NOT NULL DEFAULT '',
    started_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at   DATETIME
);
CREATE INDEX IF NOT EXISTS idx_task_runs_rule ON task_runs(rule_id, started_at);

-- 调用日志（网关请求记录）
CREATE TABLE IF NOT EXISTS request_logs (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id         INTEGER REFERENCES keys(id) ON DELETE SET NULL,
    plugin_id      INTEGER,
    account_id     INTEGER,
    model          TEXT    NOT NULL DEFAULT '',
    protocol       TEXT    NOT NULL DEFAULT '',   -- messages / chat_completions / responses
    status         INTEGER NOT NULL DEFAULT 0,    -- HTTP 状态码
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    cached_tokens  INTEGER NOT NULL DEFAULT 0,    -- 缓存命中 token
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    first_token_ms INTEGER NOT NULL DEFAULT 0,    -- 首字耗时
    client_ip      TEXT    NOT NULL DEFAULT '',
    user_agent     TEXT    NOT NULL DEFAULT '',
    error_brief    TEXT    NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_key ON request_logs(key_id, created_at);

-- 系统设置（key-value，管理账号等由 users 表承担）
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
