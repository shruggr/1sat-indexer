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
    spend BYTEA,
    vin INTEGER,
    spend_height INTEGER,
    spend_idx BIGINT,
    inacc BIGINT,
    origin BYTEA,
    data JSONB,
    PRIMARY KEY(txid, vout)
);
CREATE UNIQUE INDEX idx_txid_vout_spend ON txos(txid, vout, spend);
CREATE INDEX idx_txos_pkhash_unspent ON txos(pkhash, height NULLS LAST, idx)
    WHERE spend IS NULL AND pkhash IS NOT NULL;
CREATE INDEX idx_txo_pkhash_spent ON txos(pkhash, height NULLS LAST, idx)
    WHERE spend IS NOT NULL AND pkhash IS NOT NULL;
-- CREATE INDEX idx_txo_pkhash_spends ON txos(pkhash, spend_height, spend_idx)
--     WHERE spend IS NULL AND pkhash IS NOT NULL;
CREATE INDEX idx_txos_origin_height_idx ON txos(origin, height NULLS LAST, idx)
    WHERE origin IS NOT NULL;
-- CREATE INDEX idx_txos_data ON txos USING GIN(data)
--     WHERE spend IS NULL AND data IS NOT NULL;
CREATE INDEX idx_txos_spend ON txos(spend);
CREATE INDEX idx_txos_height_idx_vout ON txos(height, idx, vout);


CREATE TABLE origins(
    origin BYTEA PRIMARY KEY,
    txid BYTEA,
    vout INTEGER,
    num BIGINT,
    height INTEGER,
    idx BIGINT,
    map JSONB,
    search_text_en TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english',
            COALESCE(jsonb_extract_path_text(map, 'name'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'subTypeData.description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'keywords'), '')
        )
    ) STORED,
    geohash TEXT GENERATED ALWAYS AS (
        jsonb_extract_path_text(map, 'geohash')
    ) STORED
);
CREATE INDEX idx_origins_height_idx_vout ON origins(height, idx, vout)
	WHERE height IS NOT NULL AND num = -1;
CREATE INDEX idx_origins_search_text_en ON origins USING GIN(search_text_en);
CREATE INDEX idx_origins_map ON origins USING GIN(map);
CREATE INDEX idx_origins_num ON origins(num);
CREATE INDEX idx_origins_geohash ON origins(geohash text_pattern_ops)
    WHERE geohash IS NOT NULL;

-- DROP TABLE IF EXISTS blocks;
-- DROP TABLE IF EXISTS origins;
-- DROP TABLE IF EXISTS schema_migrations;
-- DROP TABLE IF EXISTS txns;
-- DROP TABLE IF EXISTS txos;
-- DROP TABLE IF EXISTS progress;

