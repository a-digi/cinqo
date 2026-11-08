/***Statement***/
-- compact_tool_results (plan/ai/conversation/step-45, step 3): 1 for a conversation whose requests fold old bulky tool results,
-- answer a repeated read-only call in one line and warn two rounds before the tool-call budget ends. NULL (every existing
-- conversation) means none of that. Set once at creation, like tool_iteration_limit.
-- (Named 31_10_2026_12_00_00 on purpose: the migrations run in file-name order, so a name that starts with a November day would
-- sort before the CREATE TABLE of conversations on a fresh install.)
ALTER TABLE conversations ADD COLUMN compact_tool_results INTEGER CHECK (compact_tool_results IS NULL OR compact_tool_results IN (0, 1));
