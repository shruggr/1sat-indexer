ALTER TABLE listings ADD COLUMN IF NOT EXISTS buyer BYTEA;
ALTER TABLE bsv20_txos ADD COLUMN IF NOT EXISTS buyer BYTEA;