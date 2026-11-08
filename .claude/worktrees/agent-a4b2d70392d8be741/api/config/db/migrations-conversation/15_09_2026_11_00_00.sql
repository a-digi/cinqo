/***Statement***/
ALTER TABLE turn_runs ADD COLUMN cancel_requested INTEGER NOT NULL DEFAULT 0;
