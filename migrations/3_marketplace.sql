CREATE TABLE IF NOT EXISTS listings(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    num BIGINT,
    price BIGINT,
    payout BYTEA,
    origin BYTEA,
    spend BYTEA,
    data JSONB,
    lock BYTEA,
    PRIMARY KEY (txid, vout) --,
    -- FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON DELETE CASCADE ON UPDATE CASCADE,
    -- FOREIGN KEY (origin, num) REFERENCES inscriptions(origin, num) ON UPDATE CASCADE
);

CREATE INDEX idx_ordinal_lock_listings_bsv20_price_unspent ON ordinal_lock_listings(bsv20, price)
WHERE spend = decode('', 'hex');

CREATE INDEX idx_ordinal_lock_listings_bsv20_num_unspent ON ordinal_lock_listings(bsv20, num)
WHERE spend = decode('', 'hex');

CREATE INDEX idx_ordinal_lock_listings_bsv20_height_idx_unspent ON ordinal_lock_listings(bsv20, height, idx)
WHERE spend = decode('', 'hex');
