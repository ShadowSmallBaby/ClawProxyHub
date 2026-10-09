DROP TABLE mcp_credentials;
DROP TABLE mcp_settings;
CREATE TABLE assistant_budgets (
 day TEXT PRIMARY KEY,
 calls INTEGER NOT NULL DEFAULT 0,
 tokens INTEGER NOT NULL DEFAULT 0,
 cost_micro INTEGER NOT NULL DEFAULT 0
);
