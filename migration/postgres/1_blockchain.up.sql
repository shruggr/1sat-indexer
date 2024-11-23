CREATE TABLE txos (
    outpoint TEXT PRIMARY KEY,
    height INT DEFAULT EXTRACT(EPOCH FROM NOW())::integer,
    idx BIGINT DEFAULT 0,
    spend TEXT NOT NULL DEFAULT '',
    satoshis BIGINT
);
CREATE INDEX idx_txos_height_idx ON txos (height, idx);
CREATE INDEX idx_txos_spend ON txos (spend);

CREATE TABLE txo_data (
    outpoint TEXT,
    tag TEXT,
    data JSONB,
    PRIMARY KEY (outpoint, tag)
);

CREATE TABLE logs (
    search_key TEXT,
    member TEXT,
    score DOUBLE PRECISION,
    PRIMARY KEY (search_key, member)
);
CREATE INDEX idx_logs_score ON logs (search_key, score);

CREATE TABLE owner_accounts (
    owner TEXT PRIMARY KEY,
    account TEXT,
    sync_height INT DEFAULT 0
);
CREATE INDEX idx_owner_accounts_account ON owner_accounts (account);

CREATE TABLE progress (
    member TEXT PRIMARY KEY,
    score DOUBLE PRECISION
);
