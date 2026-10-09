CREATE TABLE extension_storage_schemas (
    extension_id TEXT PRIMARY KEY NOT NULL,
    signer TEXT NOT NULL,
    namespace TEXT NOT NULL UNIQUE,
    schema_version INTEGER NOT NULL,
    schema_hash TEXT NOT NULL,
    schema_json TEXT NOT NULL,
    table_map TEXT NOT NULL,
    history_json TEXT NOT NULL,
    used_bytes INTEGER NOT NULL DEFAULT 0,
    applied_package TEXT NOT NULL,
    installed_package TEXT NOT NULL DEFAULT '',
    phase TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
