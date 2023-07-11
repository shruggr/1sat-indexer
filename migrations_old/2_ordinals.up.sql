
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