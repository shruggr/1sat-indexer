CREATE UNIQUE INDEX CONCURRENTLY idx_txos_outpoint ON txos (outpoint text_pattern_ops);
ALTER TABLE txo_data DROP CONSTRAINT txos_pkey;
ALTER TABLE txos ADD UNIQUE CONSTRAINT txos_pkey ON txos (outpoint);
CREATE UNIQUE INDEX CONCURRENTLY idx_txo_data_outpoint_tag ON txo_data (outpoint text_pattern_ops, tag);
-- ALTER TABLE txo_data DROP CONSTRAINT txo_data_pkey;
CREATE INDEX idx_logs_member_text ON logs (member text_pattern_ops, score);
DROP INDEX idx_logs_member;