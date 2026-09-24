-- 000012 — apikey 确定性查找列：sha256(raw) hex，鉴权 O(1) 命中，免全表解密扫描。

ALTER TABLE keys ADD COLUMN key_lookup TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_keys_lookup ON keys(key_lookup);
-- 存量 sha256 hex 密钥（key_cipher 即 sha256(raw)，长度 64）直接回填；
-- AES-GCM 密文（更长、0x01 前缀）无法回填，鉴权时回退全量解密扫描。
UPDATE keys SET key_lookup = key_cipher WHERE length(key_cipher) = 64;
