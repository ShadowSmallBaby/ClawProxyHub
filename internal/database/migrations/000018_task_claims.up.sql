ALTER TABLE task_runs ADD COLUMN execution_token TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_task_runs_token ON task_runs(execution_token);
CREATE TABLE task_claims (
 rule_id INTEGER PRIMARY KEY REFERENCES task_rules(id) ON DELETE CASCADE,
 token TEXT NOT NULL UNIQUE,
 owner TEXT NOT NULL,
 lease_until INTEGER NOT NULL
);
CREATE INDEX idx_task_claims_lease ON task_claims(lease_until);
