/***Statement***/
-- How many of a turn's tool calls the framework refused or rejected (a tool that is not available, or arguments that did not pass the
-- schema), counted live while the turn runs. A caller that works in steps (the Career CV import) reads it to know the first step did
-- not do what it was asked: a small model that cannot read the file carries on with nothing. 0 for every row from before this column.
-- (Named 31_10_2026_14_00_00 on purpose: the migrations run in file-name order, see the one before.)
-- NO SEMICOLON IN A COMMENT: the runner (coco-orm executeSQLFile) cuts the whole file at every semicolon, comments included.
ALTER TABLE turn_runs ADD COLUMN tool_errors INTEGER NOT NULL DEFAULT 0;
