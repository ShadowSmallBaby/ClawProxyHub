-- 000006 — 站内通知（任务产生的提醒，管理界面头部铃铛）

CREATE TABLE IF NOT EXISTS notifications (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT    NOT NULL DEFAULT '',
    content    TEXT    NOT NULL DEFAULT '',
    level      TEXT    NOT NULL DEFAULT 'info',
    account_id INTEGER,
    read       INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read, id);
