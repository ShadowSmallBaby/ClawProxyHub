-- UTC 时间点无需还原；日历规则由旧引擎按进程时区重新计算。
DELETE FROM settings WHERE key = 'system.timezone';
UPDATE task_rules SET next_run_at = NULL WHERE trigger_type IN ('daily', 'cron');
