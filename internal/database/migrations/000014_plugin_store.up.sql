-- 000014 — 插件 KV store 持久化（ClawHost.StoreGet/StorePut）。按插件名隔离，重启不丢。
CREATE TABLE plugin_stores (
    plugin     TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      BLOB,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plugin, key)
);
