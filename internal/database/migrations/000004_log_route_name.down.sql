-- 000004 down — 回滚调用日志路由名列

ALTER TABLE request_logs DROP COLUMN route_name;
