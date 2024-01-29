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
    dec INTEGER,
    fund_path TEXT,
    fund_pkhash BYTEA,
    fund_total NUMERIC DEFAULT 0,
    fund_used NUMERIC DEFAULT 0,
    fund_balance NUMERIC GENERATED ALWAYS AS (fund_total-fund_used) STORED
);

CREATE INDEX idx_bsv20_v2_fund_pkhash ON bsv20_v2(fund_pkhash);
CREATE INDEX idx_bsv20_v2_fund_total ON bsv20_v2(fund_total);
CREATE INDEX idx_bsv20_v2_fund_used ON bsv20_v2(fund_used);
CREATE INDEX idx_bsv20_v2_fund_balance ON bsv20_v2(fund_balance);

CREATE TABLE IF NOT EXISTS bsv20_txos(
    txid BYTEA,
    vout INT,
    height INTEGER,
    idx INTEGER,
    id BYTEA DEFAULT '\x',
	tick TEXT DEFAULT '',
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
    script BYTEA,
    implied BOOLEAN,
	PRIMARY KEY(txid, vout),
    FOREIGN KEY (txid, vout, spend) REFERENCES txos (txid, vout, spend) ON DELETE CASCADE ON UPDATE CASCADE
);

-- CREATE INDEX IF NOT EXISTS idx_bsv20_txo_validate ON bsv20_txos(op, tick, height, idx, vout)
-- WHERE status = 0;
CREATE INDEX idx_bsv20_txo_validate ON bsv20_txos(status, op, tick, height, idx, vout);


CREATE INDEX IF NOT EXISTS idx_bsv20_txo_validate_v2 ON bsv20_txos(op, id, height, idx, vout)
WHERE status = 0;

CREATE INDEX idx_bsv20_txos_id_status ON bsv20_txos(id, status)
INCLUDE(pkhash)
WHERE spend='\x';

CREATE INDEX idx_bsv20_txos_pkhash_id_status ON bsv20_txos(pkhash, id, status);

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_price_unspent ON bsv20_txos(price)
WHERE spend = '\x' AND listing = true;

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_price_per_token_unspent ON bsv20_txos(price_per_token)
WHERE spend = '\x' AND listing = true;

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_height_idx_unspent ON bsv20_txos(height, idx, vout)
WHERE spend = '\x';

CREATE TABLE bsv20v1_txns(
    txid BYTEA,
    tick TEXT DEFAULT '',
    height INTEGER,
    idx BIGINT,
    txouts INTEGER,
    created TIMESTAMP DEFAULT NOW(),
    processed BOOL DEFAULT false,
    PRIMARY KEY(txid, tick)
);
CREATE INDEX idx_bsv20v1_txns_pending ON bsv20v1_txns(tick, height, idx)
INCLUDE (txid)
WHERE processed = false;

CREATE TABLE bsv20v2_txns(
    txid BYTEA,
    id BYTEA,
    height INTEGER,
    idx BIGINT,
    txouts INTEGER,
    created TIMESTAMP DEFAULT NOW(),
    processed BOOL DEFAULT false,
    PRIMARY KEY(txid, id)
);
CREATE INDEX idx_bsv20v2_txns_pending ON bsv20v2_txns(id, height, idx)
INCLUDE (txid)
WHERE processed = false;

