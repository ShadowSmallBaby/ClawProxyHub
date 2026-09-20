-- 000003 — OAuth 平台凭据托管

-- 第三方平台（LinuxDo/GitHub 等）用户登录态：供插件换取上游 token。
-- token_blob 经核心 AES-256-GCM 加密存储（复用凭据加密密钥）。
CREATE TABLE IF NOT EXISTS oauth_credentials (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    platform      TEXT    NOT NULL,               -- 平台标识：linuxdo / github ...
    account_label TEXT    NOT NULL DEFAULT '',    -- 展示名（用户自填，区分同平台多号）
    token_blob    BLOB,                           -- 登录态（cookie/token），加密存储
    expires_at    DATETIME,                       -- 过期时间（可空 = 长期有效）
    extra_json    TEXT    NOT NULL DEFAULT '{}',  -- 平台特有附加字段
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_oauth_platform ON oauth_credentials(platform);
