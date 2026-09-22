/***Statement***/
ALTER TABLE tools ADD COLUMN service_token TEXT NOT NULL DEFAULT '';

/***Statement***/
CREATE UNIQUE INDEX IF NOT EXISTS tools_service_token_unique
    ON tools(service_token) WHERE service_token != '';
