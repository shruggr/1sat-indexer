CREATE TABLE IF NOT EXISTS bsv20_subs(
    seq BIGSERIAL PRIMARY KEY,
    name TEXT,
    id BYTEA,
    tick TEXT,
    topic TEXT,
    progress INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bsv20_subs_id ON bsv20_subs(id)
    WHERE id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bsv20_subs_tick ON bsv20_subs(tick)
    WHERE tick IS NOT NULL;

CREATE TABLE bsv20_v2(
    id BYTEA PRIMARY KEY,
    txid BYTEA GENERATED ALWAYS AS (substring(id from 1 for 32)) STORED,
    vout INTEGER GENERATED ALWAYS AS ((get_byte(id, 32) << 24) |
       (get_byte(id, 33) << 16) |
	   (get_byte(id, 34) << 8) |
       (get_byte(id, 35))) STORED,
    height INTEGER,
    idx BIGINT,
    sym TEXT,
    icon BYTEA,
    amt NUMERIC,
    dec INTEGER
);

CREATE TABLE IF NOT EXISTS bsv20_txos(
    txid BYTEA,
    vout INT,
    height INTEGER,
    idx INTEGER,
    id BYTEA,
	tick TEXT,
    op TEXT,
    amt NUMERIC,
    spend BYTEA DEFAULT '\x',
    pkhash BYTEA,
    status INTEGER DEFAULT 0,
    reason TEXT,
    listing BOOL,
    price NUMERIC,
    payout BYTEA,
    price_per_token NUMERIC,
	PRIMARY KEY(txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON DELETE CASCADE ON UPDATE CASCADE
);

-- CREATE INDEX IF NOT EXISTS idx_bsv20_txos_status ON bsv20_mints(status, height, idx, vout);

CREATE INDEX IF NOT EXISTS idx_bsv20_txo_validate ON bsv20_txos(op, tick, height, idx, vout)
WHERE status = 0;

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_price_unspent ON bsv20_txos(price)
WHERE spend = '\x' AND listing = true;

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_price_per_token_unspent ON bsv20_txos(price_per_token)
WHERE spend = '\x' AND listing = true;

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_height_idx_unspent ON bsv20_txos(height, idx, vout)
WHERE spend = '\x';