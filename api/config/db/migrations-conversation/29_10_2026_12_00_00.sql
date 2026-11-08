/***Statement***/
ALTER TABLE conversations ADD COLUMN orchestration_mode TEXT NOT NULL DEFAULT 'direct' CHECK (orchestration_mode IN ('direct','composer'));
