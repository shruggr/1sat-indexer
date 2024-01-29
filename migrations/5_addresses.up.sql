CREATE TABLE addresses (
    address CHAR(34) PRIMARY KEY,
    height INTEGER,
    updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE txn_indexer (
    txid BYTEA,
    indexer VARCHAR(64),
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (txid, indexer)
);

CREATE INDEX idx_txn_indexer_created ON txn_indexer(indexer, created);