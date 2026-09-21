-- 000008 — 路由级 User-Agent（空 = 跟随全局网关 UA；全局也空则透传客户端 UA）

ALTER TABLE routes ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
