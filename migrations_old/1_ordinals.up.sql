CREATE TABLE progress(
    indexer VARCHAR(32) PRIMARY KEY,
    height INTEGER
);

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
    block_id BYTEA,
    height INTEGER,
    idx INTEGER,
    created TIMESTAMP DEFAULT NOW()
);
CREATE INDEX txns_block_id_idx ON txns(block_id, idx);
CREATE INDEX txns_unmined ON txns(created)
    WHERE block_id IS NULL;

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
    PRIMARY KEY(txid, vout, spend)
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

CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    num BIGINT NOT NULL DEFAULT -1,
    origin BYTEA,
    ordinal BIGINT,
    height INTEGER,
    idx INTEGER,
    lock BYTEA,
    map JSONB,
    sigma JSONB,
    search_text_en TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english',
            COALESCE(jsonb_extract_path_text(map, 'name'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'subTypeData.description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'keywords'), '')
        )
    ) STORED,
    PRIMARY KEY(txid, vout)
);

CREATE UNIQUE INDEX idx_inscriptions_origin_num ON inscriptions(origin, num);
CREATE INDEX idx_inscriptions_num ON inscriptions(num);
CREATE INDEX idx_inscriptions_height_idx_vout ON inscriptions(height, idx, vout)
WHERE num = -1;
CREATE INDEX IF NOT EXISTS idx_inscriptions_map ON inscriptions USING GIN(map);
CREATE INDEX IF NOT EXISTS idx_inscriptions_search_text_en ON inscriptions USING GIN(search_text_en);
CREATE INDEX IF NOT EXISTS idx_inscriptions_sigma ON inscriptions USING GIN(sigma);

CREATE TABLE metadata(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    map JSONB,
    origin BYTEA,
    ordinal BIGINT,
    b JSONB,
    ord JSONB,
    sigma JSONB,
    PRIMARY KEY(txid, vout)
);

CREATE INDEX idx_metadata_origin_height_idx ON metadata(origin, height, idx);