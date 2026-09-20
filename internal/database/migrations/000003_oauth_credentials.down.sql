-- 000003 down — 回滚 OAuth 平台凭据

DROP INDEX IF EXISTS idx_oauth_platform;
DROP TABLE IF EXISTS oauth_credentials;
