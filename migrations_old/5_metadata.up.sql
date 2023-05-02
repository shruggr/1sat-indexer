CREATE TABLE metadata(
    txid BYTEA,
    vout INTEGER,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    map JSONB,
    origin BYTEA,
    ordinal BIGINT,
    height INTEGER,
    idx INTEGER,
    PRIMARY KEY(txid, vout)
);

CREATE INDEX idx_metadata_origin_height_idx ON metadata(origin, height, idx);
ALTER TABLE inscriptions ADD COLUMN map JSONB;