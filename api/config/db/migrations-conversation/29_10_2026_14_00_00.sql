/***Statement***/
ALTER TABLE conversations ADD COLUMN tool_iteration_limit INTEGER CHECK (tool_iteration_limit IS NULL OR tool_iteration_limit >= 1);
