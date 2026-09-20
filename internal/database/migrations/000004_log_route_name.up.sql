-- 000004 — 调用日志新增对外路由名列（model 列存真实模型，route_name 存对外路由名）

ALTER TABLE request_logs ADD COLUMN route_name TEXT NOT NULL DEFAULT '';
