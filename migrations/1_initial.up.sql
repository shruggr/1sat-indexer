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
    ordinal BIGINT,
    PRIMARY KEY (txid, idx)
);
CREATE INDEX idx_txos_spend_inacc ON txos(spend, inacc) INCLUDE (txid, outacc);
CREATE INDEX idx_txos_lock_unspent ON txos(LOCK)
    WHERE spend IS NULL;
CREATE INDEX idx_txos_lock_spent ON txos(LOCK)
    WHERE spend IS NOT NULL;

CREATE TABLE metadata(
    txid BYTEA,
    vout INTEGER,
    ord jsonb,
    map jsonb,
    b jsonb,
    -- origin BYTEA,
    ordinal BIGINT,
    height INTEGER,
    idx INTEGER,
    PRIMARY KEY (txid, vout)
);

CREATE INDEX idx_metadata_ordinal_height_idx ON metadata(ordinal, height, idx);

CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    num BIGINT,
    ordinal BIGINT,
    PRIMARY KEY(txid, vout)
);
CREATE INDEX idx_inscriptions_height_idx_vout ON inscriptions(height, idx, vout);

CREATE TABLE listings(
    txid BYTEA,
    vout INTEGER,
    spend BYTEA,
    height INTEGER,
    idx BIGINT,
    price BIGINT,
    payout BYTEA,
    ordinal DECIMAL,
    PRIMARY KEY (txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos(txid, vout, spend) ON UPDATE CASCADE
);

CREATE INDEX idx_listings_unspent ON listings(height, idx)
WHERE spend IS NULL;

CREATE INDEX idx_listings_spent ON listings(height, idx)
WHERE spend IS NOT NULL;

-- CREATE OR REPLACE FUNCTION calc_ordinal(txid BYTEA, vout INTEGER, satoshi BIGINT) RETURNS BIGINT AS $$ 
-- DECLARE ordinal BIGINT;
-- DECLARE outacc BIGINT;
-- DECLARE height INTEGER;
-- DECLARE feeacc BIGINT;
-- DECLARE subsidy BIGINT;
-- BEGIN
--     SELECT INTO txid, vout, ordinal, outacc
--     FROM txos
--     WHERE spend = txid AND inacc >= outacc
--     ORDER BY inacc ASC
--     LIMIT 1;

--     IF NOT FOUND THEN
--         subsidy := 50 * 100000000 >> height / 210000;
--         IF satoshi <= subsidy THEN
--             -- TODO: Calculate actual ordinal
--             RETURN 10000;
--         ELSE
--             -- SELECT calc_ordinal(txid, vout, outacc) INTO ordinal;
--             -- RETURN ordinal;
--         END IF; 

--         SELECT INTO height, feeacc 
--         FROM txns
--         WHERE txid = txid;
--     ELSE
--         IF ordinal > 0 THEN
--             RETURN ordinal;
--         ELSE 
--             SELECT calc_ordinal(txid, vout, outacc) INTO ordinal;
--             RETURN ordinal;
--         END IF;
--     END IF;
-- END;

-- $$ LANGUAGE plpgsql;
