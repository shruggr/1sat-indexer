-- CREATE TABLE blocks(
--     id BYTEA PRIMARY KEY,
--     height INTEGER,
--     created TIMESTAMP DEFAULT current_timestamp
-- );

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
	block_id BYTEA,
    height INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
    idx BIGINT NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX idx_txid_height_idx ON txns(txid, height, idx);
INSERT INTO txns(txid, block_id, height, idx)
VALUES ('\x', '\x', 0, 0);

CREATE TABLE txos(
    outpoint BYTEA PRIMARY KEY,
    txid BYTEA,
    vout INTEGER,
    height INTEGER NOT NULL DEFAULT EXTRACT(EPOCH FROM NOW()),
    idx BIGINT NOT NULL DEFAULT 0,
    satoshis BIGINT,
    outacc BIGINT,
    owners TEXT[],
    spend BYTEA DEFAULT '\x',
    spend_height INTEGER DEFAULT 0,
    spend_idx BIGINT DEFAULT 0,
    FOREIGN KEY(txid, height, idx) REFERENCES txns(txid, height, idx) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY(spend, spend_height, spend_idx) REFERENCES txns(txid, height, idx) ON DELETE SET NULL ON UPDATE CASCADE
);
CREATE UNIQUE INDEX idx_txos_txid_vout_spend ON txos(txid, vout, spend);
CREATE UNIQUE INDEX idx_txos_spend_txid_vout ON txos(spend, txid, vout, spend_height, spend_idx);

CREATE TABLE owners(
    owner TEXT,
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx BIGINT,
    spend BYTEA,
    spend_height INTEGER,
    spend_idx BIGINT,
    PRIMARY KEY(owner, txid, vout),
    FOREIGN KEY(txid, height, idx) REFERENCES txns(txid, height, idx) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY(spend, txid, vout, spend_height, spend_idx) 
        REFERENCES txos(spend, txid, vout, spend_height, spend_idx) 
        ON DELETE SET NULL ON UPDATE CASCADE
);
CREATE INDEX idx_owners_owner_height_idx ON owners(owner, height, idx);
CREATE INDEX idx_owners_owner_spend_height_idx ON owners(owner, spend_height, spend_idx)
WHERE spend != '\x';