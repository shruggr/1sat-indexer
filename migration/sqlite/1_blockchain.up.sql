
CREATE TABLE IF NOT EXISTS txns(
    txid TEXT PRIMARY KEY,
    height INTEGER DEFAULT (unixepoch()),
    idx BIGINT DEFAULT 0,
    outputs TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS spends(
    outpoint TEXT PRIMARY KEY,
    spend TEXT NOT NULL,
    height INTEGER NOT NULL,
    idx BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_spends_spend ON spends (spend);

CREATE TABLE IF NOT EXISTS logs (
    search_key TEXT,
    member TEXT,
    score REAL,
    PRIMARY KEY (search_key, member)
);
CREATE INDEX IF NOT EXISTS idx_logs_score ON logs (search_key, score);
CREATE INDEX IF NOT EXISTS idx_logs_member ON logs (member, score);

CREATE TABLE IF NOT EXISTS owner_accounts (
    owner TEXT PRIMARY KEY,
    account TEXT,
    sync_height INT DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_owner_accounts_account ON owner_accounts (account);
