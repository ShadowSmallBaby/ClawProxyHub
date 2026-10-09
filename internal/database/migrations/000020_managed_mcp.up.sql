CREATE TABLE mcp_settings (
 id INTEGER PRIMARY KEY CHECK (id = 1),
 enabled INTEGER NOT NULL DEFAULT 0
);
INSERT INTO mcp_settings(id, enabled) VALUES (1, 0);
CREATE TABLE mcp_credentials (
 username TEXT PRIMARY KEY REFERENCES users(username) ON DELETE CASCADE,
 token_hash TEXT UNIQUE,
 auth_version INTEGER NOT NULL DEFAULT 0,
 allowed_actions TEXT NOT NULL DEFAULT '[]',
 updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- 移除已退役的内置助手配置与预算，不影响网关账号、调用日志及插件数据。
DELETE FROM settings WHERE key = 'assistant.encrypted_config';
DROP TABLE assistant_budgets;
