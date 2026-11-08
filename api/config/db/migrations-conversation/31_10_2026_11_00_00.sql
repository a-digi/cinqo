/***Statement***/
-- allowed_tools (step 113): the MCP tools a conversation is offered and may call, as a JSON
-- array of names. NULL means no restriction (every existing conversation). Set once at creation.
ALTER TABLE conversations ADD COLUMN allowed_tools TEXT;
