-- 授权空集与全量授权分离，用户会话版本用于撤销旧令牌。
ALTER TABLE keys ADD COLUMN route_scope TEXT NOT NULL DEFAULT 'all';
UPDATE keys SET route_scope = 'restricted' WHERE EXISTS (SELECT 1 FROM key_routes WHERE key_id = keys.id);
ALTER TABLE users ADD COLUMN auth_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE proxies ADD COLUMN password_cipher BLOB;
