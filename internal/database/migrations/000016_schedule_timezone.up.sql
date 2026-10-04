-- 旧日历规则按服务器时区计算，升级后交给引擎按系统调度时区重算。
INSERT INTO settings (key, value, updated_at) VALUES ('system.timezone', 'Asia/Shanghai', CURRENT_TIMESTAMP)
ON CONFLICT(key) DO NOTHING;
UPDATE task_rules SET next_run_at = NULL WHERE trigger_type IN ('daily', 'cron');
-- 保留固定时间点，将历史显式偏移统一成 UTC，供 SQLite 按时间列比较。
UPDATE task_rules SET next_run_at = strftime('%Y-%m-%d %H:%M:%f', next_run_at)
WHERE next_run_at IS NOT NULL AND julianday(next_run_at) IS NOT NULL;
