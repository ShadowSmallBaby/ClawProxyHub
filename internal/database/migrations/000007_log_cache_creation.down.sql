-- 000007 down — 回滚调用日志缓存写入列

ALTER TABLE request_logs DROP COLUMN cache_creation_tokens;
