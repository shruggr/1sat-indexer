CREATE TABLE origins(
    origin BYTEA PRIMARY KEY,
    num BIGINT NOT NULL DEFAULT -1,
    txid BYTEA,
    vout INTEGER,
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
    ) STORED
);
CREATE UNIQUE INDEX idx_origins_num_origin ON origins(num, origin);
CREATE INDEX idx_origins_height_idx_vout ON origins(height, idx, vout)
	WHERE height IS NOT NULL AND num = -1;
CREATE INDEX idx_origins_search_text_en ON origins USING GIN(search_text_en);
CREATE INDEX idx_origins_map ON origins USING GIN(map);

CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    origin BYTEA,
    pkhash BYTEA,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    json_content JSONB,
    search_text_en TSVECTOR,
    sigma JSONB,
    PRIMARY KEY(txid, vout)
);
CREATE INDEX IF NOT EXISTS idx_inscriptions_origin ON inscriptions(origin, height NULLS LAST, idx);
CREATE INDEX IF NOT EXISTS idx_inscriptions_json_content ON inscriptions USING GIN(json_content);
CREATE INDEX IF NOT EXISTS idx_inscriptions_sigma ON inscriptions USING GIN(sigma);
CREATE INDEX IF NOT EXISTS idx_inscriptions_search_text_en ON inscriptions USING GIN(search_text_en);

CREATE TABLE map(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    origin BYTEA,
    map JSONB,
    PRIMARY KEY(txid, vout)
);
CREATE INDEX IF NOT EXISTS idx_map_origin ON map(origin, height NULLS LAST, idx);

CREATE TABLE b(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    fileenc BYTEA,
    PRIMARY KEY(txid, vout)
);
CREATE INDEX IF NOT EXISTS idx_b_filehash ON b(filehash, height NULLS LAST);