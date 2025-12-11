CREATE TABLE txos (
    outpoint TEXT PRIMARY KEY,
    height INT DEFAULT EXTRACT(EPOCH FROM NOW())::integer,
    idx BIGINT DEFAULT 0,
    spend TEXT NOT NULL DEFAULT '',
    satoshis BIGINT,
    owners TEXT[]
);
CREATE INDEX idx_txos_height_idx ON txos (height, idx);
CREATE INDEX idx_txos_spend ON txos (spend);
CREATE UNIQUE INDEX idx_txos_outpoint ON txos(outpoint text_pattern_ops);

CREATE TABLE txo_data (
    outpoint TEXT,
    tag TEXT,
    data JSONB,
    PRIMARY KEY (outpoint, tag)
);
CREATE UNIQUE INDEX idx_txo_data_outpoint_tag ON txo_data (outpoint text_pattern_ops, tag);

CREATE TABLE logs (
    search_key TEXT,
    member TEXT,
    score DOUBLE PRECISION,
    PRIMARY KEY (search_key, member)
);
CREATE INDEX idx_logs_score ON logs (search_key text_pattern_ops, score)
    INCLUDE (member);
CREATE INDEX idx_logs_member ON logs (member text_pattern_ops, score);
