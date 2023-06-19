CREATE TABLE progress(
    indexer VARCHAR(32) PRIMARY KEY,
    height INTEGER
);

CREATE TABLE blocks(
    id BYTEA PRIMARY KEY,
    height INTEGER,
    subsidy BIGINT,
    subacc BIGINT,
    fees BIGINT,
    processed TIMESTAMP
);
CREATE INDEX idx_blocks_processed ON blocks(processed);

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
	block_id BYTEA,
    height INTEGER,
    idx BIGINT,
    fees BIGINT,
    feeacc BIGINT,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX txns_block_id_feeacc_idx ON txns(block_id, feeacc, idx);
CREATE INDEX idx_txns_created_unmined ON txns(created)
    WHERE height = 0;

CREATE TABLE txos(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx BIGINT,
    satoshis BIGINT,
    outacc BIGINT,
    scripthash BYTEA,
    lock BYTEA,
    spend BYTEA DEFAULT '\x',
    vin INTEGER,
    inacc BIGINT,
    origin BYTEA,
    listing BOOLEAN DEFAULT FALSE,
    bsv20 BOOLEAN DEFAULT FALSE,
    PRIMARY KEY(txid, vout),
    FOREIGN KEY (txid) REFERENCES txns(txid) ON DELETE CASCADE
);
-- CREATE INDEX idx_txos_scripthash_unspent ON txos(scripthash, height, idx)
--     WHERE spend = '\x';
-- CREATE INDEX idx_txos_scripthash_spent ON txos(scripthash, height, idx)
--     WHERE spend != '\x';
CREATE UNIQUE INDEX idx_txid_vout_spend ON txos(txid, vout, spend);
CREATE INDEX idx_txos_lock_unspent ON txos(lock, height, idx)
    WHERE spend = '\x' AND lock IS NOT NULL;
CREATE INDEX idx_txo_lock_spent ON txos(lock, height, idx)
    WHERE spend != '\x' AND lock IS NOT NULL;
CREATE INDEX idx_txos_origin_height_idx ON txos(origin, height, idx)
    WHERE origin IS NOT NULL;