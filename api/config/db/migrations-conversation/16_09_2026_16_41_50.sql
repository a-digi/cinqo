/***Statement***/
ALTER TABLE turn_runs ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN completion_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0;
