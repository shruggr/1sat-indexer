CREATE TABLE txns(
    txid BYTEA PRIMARY KEY,
    blockid BYTEA,
    height INTEGER,
    idx INTEGER,
    first TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX txns_blockid_idx ON txns(blockid, idx);
