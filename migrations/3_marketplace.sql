CREATE TABLE IF NOT EXISTS listings(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx INTEGER,
    price BIGINT,
    payout BYTEA,
    origin BYTEA,
    num BIGINT,
    spend BYTEA DEFAULT '\x',
    pkhash BYTEA,
    map JSONB,
    bsv20 BOOLEAN,
    filetype TEXT,
    search_text_en TSVECTOR,
    PRIMARY KEY (txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON DELETE CASCADE ON UPDATE CASCADE,
    FOREIGN KEY (origin, num) REFERENCES origins(origin, num) ON UPDATE CASCADE
);

CREATE INDEX idx_listings_bsv20_price_unspent ON listings(bsv20, price)
WHERE spend = '\x';

CREATE INDEX idx_listings_bsv20_num_unspent ON listings(bsv20, num)
WHERE spend = '\x';

CREATE INDEX idx_listings_bsv20_height_idx_unspent ON listings(bsv20, height, idx)
WHERE spend = '\x';

CREATE INDEX idx_listings_filetype ON listings(filetype, height, idx)
WHERE spend = '\x' AND bsv20 = false;

CREATE INDEX idx_listings_search_text_en ON listings USING GIN(search_text_en);