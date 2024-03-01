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
    ins INTEGER,
    outs INTEGER,
    fees BIGINT,
    feeacc BIGINT,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_txns_block_id_idx ON txns(block_id, idx);
CREATE INDEX idx_txns_created_unmined ON txns(created)
    WHERE height IS NULL OR height=0;

CREATE TABLE txos(
    outpoint BYTEA PRIMARY KEY,
    txid BYTEA GENERATED ALWAYS AS (substring(outpoint from 1 for 32)) STORED,
    vout INTEGER GENERATED ALWAYS AS ((get_byte(outpoint, 32) << 24) |
       (get_byte(outpoint, 33) << 16) |
	   (get_byte(outpoint, 34) << 8) |
       (get_byte(outpoint, 35))) STORED,
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
    data JSONB,
    search_text_en TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english',
            COALESCE(jsonb_extract_path_text(data, 'insc.text'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(data, 'map.name'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(data, 'map.description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(data, 'map.subTypeData.description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(data, 'map.keywords'), '')
        )
    ) STORED,
    geohash TEXT GENERATED ALWAYS AS (
        jsonb_extract_path_text(data, 'map.geohash')
    ) STORED,
    filetype TEXT GENERATED ALWAYS AS (
        data->'insc'->'file'->>'type'
    ) STORED,
    bsv20_xfer INTEGER GENERATED ALWAYS AS (
        CASE WHEN data->'bsv20'->>'op' = 'transfer' 
        THEN CAST(data->'bsv20'->>'status' as INTEGER)
        ELSE NULL
        END
    ) STORED,
    type TEXT GENERATED ALWAYS AS (
        CASE WHEN data ? 'bsv20' THEN 'bsv20'
        ELSE CASE WHEN data ? 'insc' THEN 'insc' ELSE NULL END
        END
    ) STORED,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_txid_vout_spend ON txos(txid, vout, spend);
CREATE INDEX idx_txo_pkhash_height_idx ON txos(pkhash, height NULLS LAST, idx)
    WHERE pkhash IS NOT NULL;
CREATE INDEX idx_txos_pkhash_unspent ON txos(pkhash, height NULLS LAST, idx)
    WHERE spend = '\x' AND pkhash IS NOT NULL;
CREATE INDEX idx_txo_pkhash_spent ON txos(pkhash, height NULLS LAST, idx)
    WHERE spend != '\x' AND pkhash IS NOT NULL;

CREATE INDEX idx_txos_origin_height_idx ON txos(origin, height NULLS LAST, idx)
    WHERE origin IS NOT NULL;
CREATE INDEX idx_txos_spend ON txos(spend)
    WHERE spend != '\x';
CREATE INDEX idx_txos_height_idx_vout ON txos(height NULLS LAST, idx, vout);
CREATE INDEX idx_txos_data ON txos USING GIN(data)
    WHERE data IS NOT NULL;
CREATE INDEX idx_txos_data_unspent ON txos USING GIN(data)
    WHERE data IS NOT NULL AND spend = '\x';
CREATE INDEX idx_txos_unmined ON txos(created)
    WHERE height IS NULL;
-- CREATE INDEX idx_txos_search_text_en ON txos USING GIN(search_text_en);
-- CREATE INDEX idx_txos_geohash ON txos(geohash text_pattern_ops)
--     WHERE geohash IS NOT NULL;
-- CREATE INDEX idx_txos_filetype ON txos(filetype text_pattern_ops, height NULLS LAST, idx)
--     WHERE filetype IS NOT NULL;
CREATE INDEX idx_txos_bsv20_xfer_height_idx ON txos(bsv20_xfer, height, idx)
    WHERE bsv20_xfer IS NOT NULL;
CREATE INDEX idx_txos_type_height_idx ON txos(type, height NULLS LAST, idx)
    WHERE type IS NOT NULL;

CREATE TABLE inscriptions(
    height INTEGER,
    idx BIGINT,
    vout INTEGER,
    num BIGSERIAL,
    PRIMARY KEY(height, idx, vout)
);
CREATE INDEX idx_inscriptions_num ON inscriptions(num, height, idx, vout);
    
CREATE TABLE origins(
    origin BYTEA PRIMARY KEY,
    map JSONB
);

