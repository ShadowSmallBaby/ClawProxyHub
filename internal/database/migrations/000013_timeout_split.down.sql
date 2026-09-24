-- 回滚：首帧+首字 合回单一 timeout_seconds（取首字值），设置键还原回 first_event。

ALTER TABLE routes ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;
UPDATE routes SET timeout_seconds = first_token_timeout_seconds;
ALTER TABLE routes DROP COLUMN first_token_timeout_seconds;
ALTER TABLE routes DROP COLUMN first_event_timeout_seconds;

INSERT INTO settings (key, value, updated_at)
    SELECT 'gateway.first_event_timeout', value, CURRENT_TIMESTAMP
    FROM settings WHERE key = 'gateway.first_token_timeout'
    ON CONFLICT(key) DO UPDATE SET value = excluded.value;
DELETE FROM settings WHERE key = 'gateway.first_token_timeout';
