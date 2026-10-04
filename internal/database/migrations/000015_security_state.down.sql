-- 受限空集降级后会变为全授权，先禁用这些密钥。密文不能降级成明文。
UPDATE keys SET enabled = 0 WHERE route_scope = 'restricted' AND NOT EXISTS (SELECT 1 FROM key_routes WHERE key_id = keys.id);
CREATE TEMP TABLE cph_rollback_guard (ok INTEGER CHECK (ok = 1));
INSERT INTO cph_rollback_guard SELECT 0 WHERE EXISTS (SELECT 1 FROM proxies WHERE length(password_cipher) > 0);
DROP TABLE cph_rollback_guard;
ALTER TABLE proxies DROP COLUMN password_cipher;
ALTER TABLE users DROP COLUMN auth_version;
ALTER TABLE keys DROP COLUMN route_scope;
