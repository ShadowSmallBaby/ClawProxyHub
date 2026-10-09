CREATE TABLE action_audits (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 action_id TEXT NOT NULL,
 owner TEXT NOT NULL,
 subject TEXT NOT NULL,
 effect TEXT NOT NULL,
 started_at DATETIME NOT NULL,
 duration_ms INTEGER NOT NULL,
 outcome TEXT NOT NULL
);
CREATE INDEX idx_action_audits_started ON action_audits(started_at);
