-- 000010 — 运行日志（系统级：账号刷新/登录失败、插件日志、核心内部事件）

CREATE TABLE IF NOT EXISTS run_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    level      TEXT    NOT NULL DEFAULT 'error',  -- error / warn / debug / info（递增包含：配置为某级则记录该级及更严重）
    module     TEXT    NOT NULL DEFAULT '',       -- 功能模块（account / plugin / task / gateway …）
    action     TEXT    NOT NULL DEFAULT '',       -- 操作（refresh / login / 插件名 …）
    message    TEXT    NOT NULL DEFAULT '',       -- 精简消息（友好提示）
    detail     TEXT    NOT NULL DEFAULT '',       -- 调试明细（上游响应体 / err 全文）
    account_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_run_logs_created ON run_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_run_logs_module ON run_logs(module, id);
