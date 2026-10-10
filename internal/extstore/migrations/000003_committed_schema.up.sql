ALTER TABLE extension_storage_schemas ADD COLUMN active_schema TEXT NOT NULL DEFAULT 'null';
ALTER TABLE extension_storage_schemas ADD COLUMN committed_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE extension_storage_schemas ADD COLUMN obsolete_json TEXT NOT NULL DEFAULT '[]';
