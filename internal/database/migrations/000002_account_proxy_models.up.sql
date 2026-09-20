-- 000002 — 账号级出站代理绑定 + 账号模型目录快照

CREATE TABLE IF NOT EXISTS account_proxies (
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    proxy_id   INTEGER NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    PRIMARY KEY (account_id, proxy_id)
);

ALTER TABLE accounts ADD COLUMN models_json TEXT NOT NULL DEFAULT '';
