/***Statement***/
-- What the provider reported for a turn besides the prompt and completion counts (plan/ai/conversation/step-178-provider-usage-openrouter.md,
-- step 2): the prompt cache (read and write), the hidden reasoning tokens (inside completion_tokens) and the cost. The counts are 0 for
-- every row from before these columns (and for a provider that reports none). cost_microusd is whole micro-dollars (1,000,000 = 1 US
-- dollar) so sums never drift. NULL means no response of the turn carried a cost (unknown), 0 means the provider said the calls were free.
-- (Named 31_10_2026_13_00_00 on purpose: the migrations run in file-name order, so a name that starts with a November day would sort
-- before the CREATE TABLE of turn_runs on a fresh install.)
-- NO SEMICOLON IN A COMMENT: the runner (coco-orm executeSQLFile) cuts the whole file at every semicolon, comments included.
ALTER TABLE turn_runs ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN reasoning_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN cost_microusd INTEGER;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN reasoning_tokens INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE sub_agent_runs ADD COLUMN cost_microusd INTEGER;
