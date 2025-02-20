CREATE UNIQUE INDEX idx_txos_outpoint ON txos(outpoint text_pattern_ops);
CREATE UNIQUE INDEX idx_txo_data_outpoint_tag ON txo_data (outpoint text_pattern_ops, tag);
CREATE INDEX idx_logs_member ON logs (member text_pattern_ops, score);
DROP INDEX idx_logs_member;