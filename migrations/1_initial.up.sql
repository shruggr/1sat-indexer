CREATE TABLE progress(
    indexer VARCHAR(32) PRIMARY KEY,
    height INTEGER
);

CREATE TABLE blocks(
    id BYTEA PRIMARY KEY,
    height INTEGER,
    subsidy BIGINT,
    subacc BIGINT
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
CREATE INDEX idx_txos_lock_unspent ON txos(lock, height, idx)
    WHERE spend = '\x' AND lock IS NOT NULL;
CREATE INDEX idx_txo_lock_spent ON txos(lock, height, idx)
    WHERE spend != '\x' AND lock IS NOT NULL;
CREATE INDEX idx_txos_origin_height_idx ON txos(origin, height, idx)
    WHERE origin IS NOT NULL;

CREATE TABLE origins(
    origin BYTEA PRIMARY KEY,
    height INTEGER,
    idx BIGINT,
    num BIGINT NOT NULL DEFAULT -1,
    -- ordinal NOT NULL DEFAULT -1,
    map JSONB,
    search_text_en TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english',
            COALESCE(jsonb_extract_path_text(map, 'name'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'subTypeData.description'), '') || ' ' || 
            COALESCE(jsonb_extract_path_text(map, 'keywords'), '')
        )
    ) STORED,
);
CREATE UNIQUE INDEX idx_num_origin ON origins(num, origin);
CREATE INDEX idx_origins_height_idx_vout ON origins(height, idx, vout)
WHERE num = -1;

CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    origin BYTEA,
    num BIGINT NOT NULL DEFAULT -1,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    json_content JSONB,
    sigma JSONB,
    PRIMARY KEY(txid, vout),
    FOREIGN KEY (txid) REFERENCES txns(txid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_inscriptions_origin ON inscriptions(origin, height, idx);
CREATE INDEX IF NOT EXISTS idx_inscriptions_json_content ON inscriptions USING GIN(json_content);
CREATE INDEX IF NOT EXISTS idx_inscriptions_search_text_en ON inscriptions USING GIN(search_text_en);
CREATE INDEX IF NOT EXISTS idx_inscriptions_map ON inscriptions USING GIN(map);
CREATE INDEX IF NOT EXISTS idx_inscriptions_sigma ON inscriptions USING GIN(sigma);

CREATE TABLE map(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    origin BYTEA,
    map JSONB,
    PRIMARY KEY(txid, vout),
    FOREIGN KEY (txid) REFERENCES txns(txid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_map_origin ON map(origin, height, idx);
-- CREATE INDEX IF NOT EXISTS idx_map_search_text_en ON map USING GIN(search_text_en);
-- CREATE INDEX IF NOT EXISTS idx_map_map ON map USING GIN(map);

-- CREATE TABLE files(
--     txid BYTEA,
--     vout INTEGER,
--     height INTEGER,
--     idx INTEGER,
--     filehash BYTEA,
--     filesize INTEGER,
--     filetype BYTEA,
--     fileenc BYTEA,
--     PRIMARY KEY(txid, vout)
-- );
-- CREATE INDEX idx_files_filehash ON files(filehash);

-- CREATE TABLE events(
--     id BYTEA,
--     channel TEXT,

--     created TIMESTAMP DEFAULT
-- )