/***Statement***/
CREATE TABLE IF NOT EXISTS turn_plan_steps (
    turn_run_id      TEXT NOT NULL REFERENCES turn_runs(id) ON DELETE CASCADE,
    plan_no          INTEGER NOT NULL,
    step_id          TEXT NOT NULL,
    task             TEXT NOT NULL,
    depends_on       TEXT NOT NULL DEFAULT '[]',
    status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','skipped')),
    sub_agent_run_id TEXT REFERENCES sub_agent_runs(id) ON DELETE SET NULL,
    PRIMARY KEY (turn_run_id, plan_no, step_id)
);
