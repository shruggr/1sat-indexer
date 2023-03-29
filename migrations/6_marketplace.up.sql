ALTER TABLE txos ADD COLUMN listing BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_txos_listing_spend_height ON txos (listing, spend, height, idx);

CREATE TABLE listings(
    ltxid BYTEA,
    lvout INTEGER,
    lseq INTEGER,
    height INTEGER,
    idx INTEGER,
    txid BYTEA,
    vout INTEGER,
    price BIGINT,
    rawtx BYTEA,
    origin BYTEA,
    PRIMARY KEY (ltxid, lvout, lseq)
);


ALTER TABLE metadata ADD COLUMN b JSONB;
ALTER TABLE metadata ADD COLUMN ord JSONB;
ALTER TABLE metadata DROP COLUMN filehash;
ALTER TABLE metadata DROP COLUMN filesize;
ALTER TABLE metadata DROP COLUMN filetype;
