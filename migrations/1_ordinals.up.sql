CREATE TABLE progress(
    indexer VARCHAR(32) PRIMARY KEY,
    height INTEGER
);

CREATE TABLE txos(
    txid BYTEA,
    vout INTEGER,
	satoshis BIGINT,
    acc_sats BIGINT,
	lock BYTEA,
	spend BYTEA,
	vin INTEGER,
	origin BYTEA,
    ordinal BIGINT,
	PRIMARY KEY(txid, vout)
);
CREATE INDEX idx_txos_lock ON txos(lock, spend);
CREATE INDEX idx_txos_spend_vin ON txos(spend, vin);

CREATE TABLE inscriptions(
    txid BYTEA,
    vout INTEGER,
    filehash BYTEA,
    filesize INTEGER,
    filetype BYTEA,
    id BIGINT,
    origin BYTEA,
    ordinal BIGINT,
    height INTEGER,
    idx INTEGER,
    lock BYTEA,
    PRIMARY KEY(txid, vout)
);

-- CREATE INDEX idx_inscriptions_id
-- ON inscriptions(id);

-- CREATE INDEX idx_inscriptions_origin
-- ON inscriptions(origin);

-- CREATE INDEX idx_inscriptions_ordinal
-- ON inscriptions(ordinal);

-- CREATE INDEX idx_inscriptions_filehash
-- ON inscriptions(filehash);