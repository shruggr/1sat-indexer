CREATE TABLE progress(
    indexer VARCHAR(32) PRIMARY KEY,
    height INTEGER
);

CREATE TABLE blocks(
    id BYTEA PRIMARY KEY,
    height INTEGER,
    subsidy BIGINT,
    subacc BIGINT,
);

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
    height INTEGER,
    idx BIGINT,
    fees BIGINT,
    feeacc BIGINT
);
CREATE INDEX txns_blockid_feeacc ON txns(blockid, feeacc);

CREATE TABLE txos(
    txid BYTEA,
    idx INTEGER,
    satoshis BIGINT,
    outacc BIGINT,
    lock BYTEA,
    spend BYTEA,
    inacc BIGINT,
    ordinal DECIMAL,
    PRIMARY KEY (txid, idx)
);
CREATE INDEX idx_txos_spend_inacc ON txos(spend, inacc) INCLUDE (txid, outacc);
CREATE INDEX idx_txos_lock_unspent ON txos(LOCK)
    WHERE spend IS NULL;
CREATE INDEX idx_txos_lock_spent ON txos(LOCK)
    WHERE spend IS NOT NULL;

-- CREATE TABLE onesat(
--     txid BYTEA,
--     vout INTEGER,
--     spend BYTEA,
-- 	lock BYTEA,
--     listing BOOLEAN,
--     ordinal BIGINT,
-- 	PRIMARY KEY(txid, vout),
--     FOREIGN KEY(txid, vout, spend) REFERENCES txos(txid, vout, spend) ON UPDATE CASCADE
-- );
-- CREATE INDEX idx_txos_lock_unspent ON txos(lock)
-- WHERE spend IS NULL;
-- CREATE INDEX idx_txos_lock_spent ON txos(lock)
-- WHERE spend IS NOT NULL;


CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    num BIGINT,
    ordinal BIGINT,
    height INTEGER,
    idx INTEGER,
    PRIMARY KEY(txid, vout)
);
CREATE INDEX idx_inscriptions_height_idx_vout ON inscriptions(height, idx, vout);
