CREATE TABLE origins(
    origin BYTEA PRIMARY KEY,
    vout INTEGER,
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
    ) STORED
);
CREATE UNIQUE INDEX idx_origins_num_origin ON origins(num, origin);
CREATE INDEX idx_origins_height_idx_vout ON origins(height, idx, vout)
	WHERE num = -1;
CREATE INDEX idx_origins_search_text_en ON origins USING GIN(search_text_en);
CREATE INDEX idx_origins_map ON origins USING GIN(map);


CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    origin BYTEA,
    -- num BIGINT NOT NULL DEFAULT -1,
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

CREATE TABLE b(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    fileenc BYTEA,
    PRIMARY KEY(txid, vout),
)