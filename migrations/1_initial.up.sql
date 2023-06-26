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
CREATE INDEX idx_blocks_height_unprocessed ON blocks(height)
    WHERE processed IS NULL;

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
	block_id BYTEA,
    height INTEGER,
    idx BIGINT,
    fees BIGINT,
    feeacc BIGINT,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
-- CREATE INDEX idx_txns_block_id_feeacc_idx ON txns(block_id, feeacc, idx);
CREATE INDEX idx_txns_block_id_idx ON txns(block_id, idx);
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
    PRIMARY KEY(txid, vout, spend),
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

CREATE OR REPLACE FUNCTION fn_acc_fees(max_height INTEGER)
RETURNS INTEGER LANGUAGE plpgsql AS $$
DECLARE
    blocks CURSOR(max_height INTEGER) FOR
        SELECT id, height 
		FROM blocks 
		WHERE processed IS NULL AND height <= max_height
		ORDER BY height;
    txns CURSOR(id BYTEA) FOR
        SELECT txid, fees
        FROM txns
        WHERE block_id=id
        ORDER BY idx;
    block_id BYTEA;
    height INTEGER;
    accfees BIGINT;
    currentid BYTEA;
    fees BIGINT;
BEGIN
    OPEN blocks(max_height);
    LOOP
        FETCH blocks INTO block_id, height;
        EXIT WHEN NOT FOUND;
		accfees = 0;
        OPEN txns(block_id);
        LOOP
            FETCH txns INTO currentid, fees;
            EXIT WHEN NOT FOUND;
            UPDATE txns SET feeacc=accfees WHERE txid=currentid;
            accfees = accfees + fees;
        END LOOP;
		CLOSE txns;
		
		UPDATE blocks 
		SET processed=CURRENT_TIMESTAMP
		WHERE id = block_id;
        
		RAISE NOTICE 'Processed Block: % % %', height, block_id, accfees;
    END LOOP;
    CLOSE blocks;
    RETURN height;
END;
$$