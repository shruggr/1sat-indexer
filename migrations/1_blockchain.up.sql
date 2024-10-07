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
CREATE UNIQUE INDEX idx_txid_block_id_idx ON txns(txid, height, idx);
CREATE INDEX idx_txns_block_id_idx ON txns(block_id, idx);
CREATE INDEX idx_txns_height_idx ON txns(height, idx);

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
    spend_height INTEGER,
    spend_idx BIGINT,
    FOREIGN KEY(txid, height, idx) REFERENCES txns(txid, height, idx) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY(spend, spend_height, spend_idx) REFERENCES txns(txid, height, idx) ON DELETE SET NULL ON UPDATE CASCADE
);
CREATE UNIQUE INDEX idx_txid_vout_spend ON txos(txid, vout, spend);
-- CREATE INDEX idx_txo_pkhash_height_idx ON txos(pkhash, height NULLS LAST, idx)
--     WHERE pkhash IS NOT NULL;
-- CREATE INDEX idx_txos_pkhash_unspent ON txos(pkhash, height NULLS LAST, idx)
--     WHERE spend = '\x' AND pkhash IS NOT NULL;
-- CREATE INDEX idx_txo_pkhash_spent ON txos(pkhash, height NULLS LAST, idx)
--     WHERE spend != '\x' AND pkhash IS NOT NULL;

-- CREATE INDEX idx_txos_origin_height_idx ON txos(origin, height NULLS LAST, idx)
--     WHERE origin IS NOT NULL;
-- CREATE INDEX idx_txos_spend ON txos(spend)
--     WHERE spend != '\x';
-- CREATE INDEX idx_txos_height_idx_vout ON txos(height NULLS LAST, idx, vout);
-- CREATE INDEX idx_txos_data ON txos USING GIN(data)
--     WHERE data IS NOT NULL;
-- CREATE INDEX idx_txos_data_unspent ON txos USING GIN(data)
--     WHERE data IS NOT NULL AND spend = '\x';
-- CREATE INDEX idx_txos_unmined ON txos(created)
--     WHERE height IS NULL;
-- -- CREATE INDEX idx_txos_search_text_en ON txos USING GIN(search_text_en);
-- -- CREATE INDEX idx_txos_geohash ON txos(geohash text_pattern_ops)
-- --     WHERE geohash IS NOT NULL;
-- -- CREATE INDEX idx_txos_filetype ON txos(filetype text_pattern_ops, height NULLS LAST, idx)
-- --     WHERE filetype IS NOT NULL;
-- CREATE INDEX idx_txos_bsv20_xfer_height_idx ON txos(bsv20_xfer, height, idx)
--     WHERE bsv20_xfer IS NOT NULL;
-- CREATE INDEX idx_txos_type_height_idx ON txos(type, height NULLS LAST, idx)
--     WHERE type IS NOT NULL;

-- CREATE TABLE inscriptions(
--     height INTEGER,
--     idx BIGINT,
--     vout INTEGER,
--     num BIGSERIAL,
--     PRIMARY KEY(height, idx, vout)
-- );
-- CREATE INDEX idx_inscriptions_num ON inscriptions(num, height, idx, vout);
    
-- CREATE TABLE origins(
--     origin BYTEA PRIMARY KEY,
--     map JSONB
-- );

