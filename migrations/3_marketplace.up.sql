CREATE TABLE IF NOT EXISTS listings(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    price BIGINT,
    payout BYTEA,
    origin BYTEA,
    num BIGINT,
    spend BYTEA,
    pkhash BYTEA,
    map JSONB,
    bsv20 BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON UPDATE CASCADE,
    FOREIGN KEY (num, origin) REFERENCES origins(num, origin) ON UPDATE CASCADE
);
CREATE INDEX idx_listings_bsv20_price_unspent ON listings(bsv20, price)
    WHERE spend = '\x';
CREATE INDEX idx_listings_bsv20_num_unspent ON listings(bsv20, num)
    WHERE spend = '\x';
CREATE INDEX idx_listings_bsv20_height_idx_unspent ON listings(bsv20, height, idx)
    WHERE spend = '\x';
CREATE INDEX idx_listings_map ON listings USING GIN (map)
    WHERE spend = '\x';
