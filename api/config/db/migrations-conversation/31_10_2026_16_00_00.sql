-- Per-turn settings of a message (plan/ai/small-llm-local/step-210-import-progress-in-the-chat.md, step 1). fresh_context is 1 when the turn sends the
-- model only the new message and none of the conversation's earlier turns, 0 otherwise. allowed_tools is the JSON list of the tools this turn is offered,
-- NULL for the conversation's own list. Both are set only when the turn starts (from the message), kept for a retry of the turn, and never returned by
-- any endpoint. 0 and NULL for every row from before these columns.
-- (Named 31_10_2026_16_00_00 on purpose: the migrations run in file-name order, so a name that starts with a November day would sort before the CREATE
-- TABLE of turn_runs on a fresh install.)
-- NO SEMICOLON IN A COMMENT: the runner (coco-orm executeSQLFile) cuts the whole file at every semicolon, comments included.
ALTER TABLE turn_runs ADD COLUMN fresh_context INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE turn_runs ADD COLUMN allowed_tools TEXT;
