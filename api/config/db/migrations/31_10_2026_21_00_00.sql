-- Local AI is managed by Cinqo only: the own-server mode and its address are gone, and so is the choice of mode.
-- The model is now the id of a catalog model, kept in the existing model_id column.
-- This file must not contain a semicolon inside a comment, because the migration runner cuts a file at every one.
ALTER TABLE local_ai_settings DROP COLUMN external_base_url;

ALTER TABLE local_ai_settings DROP COLUMN mode;
