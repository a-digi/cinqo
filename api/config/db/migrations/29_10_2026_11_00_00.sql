/***Statement***/
ALTER TABLE tool_mcp_tools ADD COLUMN can_delegate INTEGER NOT NULL DEFAULT 0;
/***Statement***/
ALTER TABLE tool_mcp_tools ADD COLUMN delegate_allowed_tools TEXT NOT NULL DEFAULT '[]';
