/***Statement***/
-- Retrying the last step of a failed turn (plan/ai/conversation/retry-last-step.md, step 3). checkpoint is the saved state of the turn's
-- tool loop (the messages of the call that failed, as JSON), NULL when there is none: it is never returned by any endpoint and is
-- cleared when the conversation's next turn starts or the conversation is deleted. retry_of is the failed turn run this run retries,
-- NULL for a fresh turn. retry_count is the place in that chain (0 for a fresh turn, 1 for the first retry). 0 and NULL for every row
-- from before these columns.
-- (Named 31_10_2026_15_00_00 on purpose: the migrations run in file-name order, so a name that starts with a November day would sort
-- before the CREATE TABLE of turn_runs on a fresh install.)
-- NO SEMICOLON IN A COMMENT: the runner (coco-orm executeSQLFile) cuts the whole file at every semicolon, comments included.
ALTER TABLE turn_runs ADD COLUMN checkpoint TEXT;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN retry_of TEXT;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
