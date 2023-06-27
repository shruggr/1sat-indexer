CREATE TABLE progress(
    indexer VARCHAR(32) PRIMARY KEY,
    height INTEGER
);

CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
    blockid BYTEA,
    height INTEGER,
    idx INTEGER,
    first TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX txns_blockid_idx ON txns(blockid, idx);

CREATE TABLE txos(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx BIGINT,
	satoshis BIGINT,
    acc_sats BIGINT,
	lock BYTEA,
	spend BYTEA NOT NULL DEFAULT decode('', 'hex'),
	vin INTEGER,
	origin BYTEA,
    ordinal BIGINT,
    listing BOOLEAN NOT NULL DEFAULT FALSE,
    bsv20 BOOLEAN NOT NULL DEFAULT FALSE,
    created TIMESTAMP DEFAULT NOW(),
	PRIMARY KEY(txid, vout)
);
CREATE INDEX idx_txos_lock_spend_height_idx ON txos(lock, spend, height, idx);
CREATE INDEX idx_txos_spend_vin ON txos(spend, vin);
CREATE INDEX idx_txos_listing_spend_height ON txos (listing, spend, height DESC, idx DESC);
CREATE UNIQUE INDEX idx_txos_txid_vout_spend ON txos(txid, vout, spend);
CREATE INDEX IF NOT EXISTS txos_unmined ON txos(created)
WHERE height=2147483647;

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