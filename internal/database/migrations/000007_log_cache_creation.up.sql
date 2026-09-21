-- 000007 — 调用日志新增缓存写入 token 列（cached_tokens 为缓存读取；input_tokens 为非缓存输入）

ALTER TABLE request_logs ADD COLUMN cache_creation_tokens INTEGER NOT NULL DEFAULT 0;
