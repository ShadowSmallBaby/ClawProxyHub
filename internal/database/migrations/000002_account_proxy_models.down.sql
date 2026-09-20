-- 000002 回滚（子表在前）
ALTER TABLE accounts DROP COLUMN models_json;
DROP TABLE IF EXISTS account_proxies;
