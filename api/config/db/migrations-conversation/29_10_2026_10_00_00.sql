/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN acceptance_criteria TEXT;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 1;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN attempts INTEGER NOT NULL DEFAULT 1;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN accepted INTEGER;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN acceptance_feedback TEXT;
