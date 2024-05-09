CREATE TABLE IF NOT EXISTS listings(
    txid BYTEA,
    vout INTEGER,
    height INTEGER,
    idx BIGINT,
    price NUMERIC,
    payout BYTEA,
    origin BYTEA,
    oheight INTEGER,
    oidx BIGINT,
    spend BYTEA DEFAULT '\x',
    pkhash BYTEA,
    data JSONB,
    bsv20 BOOLEAN,
    filetype TEXT GENERATED ALWAYS AS (
        data->'insc'->'file'->>'type'
    ) STORED,
    search_text_en TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english',
            COALESCE(data->'insc'->>'text', '') || ' ' || 
            COALESCE(data->'map'->>'name', '') || ' ' || 
            COALESCE(data->'map'->>'description', '') || ' ' || 
            COALESCE(data->'map'->'subTypeData'->>'description', '') || ' ' || 
            COALESCE(data->'map'->>'keywords', '')
        )
    ) STORED,
    PRIMARY KEY (txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX idx_listings_bsv20_price_unspent ON listings(bsv20, price)
WHERE spend = '\x';

-- CREATE INDEX idx_listings_bsv20_num_unspent ON listings(bsv20, oheight, oidx, vout)
-- WHERE spend = '\x';

CREATE INDEX idx_listings_num_unspent ON listings(oheight, oidx, vout)
WHERE spend = '\x';

CREATE INDEX idx_listings_bsv20_height_idx_unspent ON listings(bsv20, height, idx)
WHERE spend = '\x';

CREATE INDEX idx_listings_filetype ON listings(filetype, height, idx)
WHERE spend = '\x' AND bsv20 = false;

CREATE INDEX idx_listings_search_text_en ON listings USING GIN(search_text_en)
WHERE spend = '\x' AND bsv20 = false;

CREATE INDEX idx_listings_data ON listings USING GIN(data)
WHERE spend = '\x' AND bsv20 = false;