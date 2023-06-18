CREATE TABLE IF NOT EXISTS ordinal_lock_listings(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    price BIGINT,
    payout BYTEA,
    origin BYTEA,
    num BIGINT,
    spend BYTEA,
    lock BYTEA,
    map JSONB,
    bsv20 BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON UPDATE CASCADE,
    FOREIGN KEY (num, origin) REFERENCES origins(num, origin) ON UPDATE CASCADE,
    FOREIGN KEY (txid) REFERENCES txns(txid) ON DELETE CASCADE
);
CREATE INDEX idx_ordinal_lock_listings_bsv20_price_unspent ON ordinal_lock_listings(bsv20, price)
    WHERE spend = '\x';
CREATE INDEX idx_ordinal_lock_listings_bsv20_num_unspent ON ordinal_lock_listings(bsv20, num)
    WHERE spend = '\x';
CREATE INDEX idx_ordinal_lock_listings_bsv20_height_idx_unspent ON ordinal_lock_listings(bsv20, height, idx)
    WHERE spend = '\x';
CREATE INDEX idx_ordinal_lock_listings_map ON ordinal_lock_listings USING GIN (map)
    WHERE spend = '\x';
