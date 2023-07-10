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
    processed TIMESTAMP DEFAULT current_timestamp
);

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
	block_id BYTEA,
    height INTEGER,
    idx BIGINT,
    fees BIGINT,
    feeacc BIGINT,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_txns_block_id_idx ON txns(block_id, idx);
CREATE INDEX idx_txns_created_unmined ON txns(created)
    WHERE height IS NULL;

CREATE TABLE txos(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx BIGINT,
    satoshis BIGINT,
    outacc BIGINT,
    pkhash BYTEA,
    spend BYTEA DEFAULT '\x',
    vin INTEGER,
    spend_height INTEGER,
    spend_idx BIGINT,
    inacc BIGINT,
    origin BYTEA,
    listing BOOLEAN DEFAULT FALSE,
    bsv20 BOOLEAN DEFAULT FALSE,
    PRIMARY KEY(txid, vout)
);
CREATE UNIQUE INDEX idx_txid_vout_spend ON txos(txid, vout, spend);
CREATE INDEX idx_txos_pkhash_unspent ON txos(pkhash, height NULLS LAST, idx)
    WHERE spend = '\x' AND pkhash IS NOT NULL;
CREATE INDEX idx_txo_pkhash_spent ON txos(pkhash, height NULLS LAST, idx)
    WHERE spend != '\x' AND  pkhash IS NOT NULL;
CREATE INDEX idx_txo_pkhash_spends ON txos(pkhash, spend_height, spend_idx)
    WHERE spend != '\x' AND  pkhash IS NOT NULL;
CREATE INDEX idx_txos_origin_height_idx ON txos(origin, height NULLS LAST, idx)
    WHERE origin IS NOT NULL;