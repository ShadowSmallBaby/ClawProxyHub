DROP TABLE IF EXISTS task_claims;
DROP INDEX IF EXISTS idx_task_runs_token;
ALTER TABLE task_runs DROP COLUMN execution_token;
