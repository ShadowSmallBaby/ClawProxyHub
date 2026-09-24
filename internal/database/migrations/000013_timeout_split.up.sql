-- 000013 — 网关超时体系拆分为「首帧 + 首字」，各自路由级可覆盖全局。
-- 旧语义：gateway.first_event_timeout / routes.timeout_seconds 实际都在管「等到首个内容」，
-- 故其值迁入首字(first_token)，首帧(first_event)为新引入概念、走默认。

-- 设置 KV：旧 first_event 值语义即首字，迁到 first_token
INSERT INTO settings (key, value, updated_at)
    SELECT 'gateway.first_token_timeout', value, CURRENT_TIMESTAMP
    FROM settings WHERE key = 'gateway.first_event_timeout'
    ON CONFLICT(key) DO UPDATE SET value = excluded.value;
DELETE FROM settings WHERE key = 'gateway.first_event_timeout';

-- 路由：timeout_seconds(语义即首字) 拆成 首帧 + 首字，值迁入首字后废弃旧列
ALTER TABLE routes ADD COLUMN first_event_timeout_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE routes ADD COLUMN first_token_timeout_seconds INTEGER NOT NULL DEFAULT 0;
UPDATE routes SET first_token_timeout_seconds = timeout_seconds;
ALTER TABLE routes DROP COLUMN timeout_seconds;
