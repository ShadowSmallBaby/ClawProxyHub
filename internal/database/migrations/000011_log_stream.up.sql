-- 000011 — 请求日志加 stream 列（同步/流式）

ALTER TABLE request_logs ADD COLUMN stream INTEGER NOT NULL DEFAULT 0;
