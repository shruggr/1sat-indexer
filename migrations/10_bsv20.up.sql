ALTER TABLE txos ADD COLUMN IF NOT EXISTS bsv20 BOOL;
CREATE TABLE IF NOT EXISTS bsv20 (
    id BYTEA PRIMARY KEY,
    height INT,
    idx BIGINT,
    tick TEXT,
    max BIGINT NOT NULL,
    lim BIGINT NOT NULL,
    dec INT DEFAULT 18,
    supply BIGINT DEFAULT 0,
    map JSONB,
    b JSONB,
    valid BOOL
);
CREATE INDEX IF NOT EXISTS idx_bsv20_tick ON bsv20(tick);
-- CREATE INDEX IN NOT EXISTS idx_bsv20_valid_height_idx ON bsv20(valid, height, idx, vout);
-- CREATE INDEX IN NOT EXISTS idx_bsv20_height_idx ON bsv20(height, idx);

CREATE TABLE IF NOT EXISTS bsv20_txos (
    txid BYTEA,
    vout INT,
    height INT,
    idx BIGINT,
    op TEXT,
    tick TEXT,
    id BYTEA,
    amt BIGINT NOT NULL,
    lock BYTEA,
    spend BYTEA,
    valid BOOL,
    bsv20 BOOL DEFAULT FALSE,
    PRIMARY KEY(txid, vout),
    FOREIGN KEY(txid, vout, spend) REFERENCES txos(txid, vout, spend) ON UPDATE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_lock ON bsv20_txos(lock, valid, spend);
CREATE INDEX IF NOT EXISTS idx_bsv20_txos_spend ON bsv20_txos(spend);
CREATE INDEX IF NOT EXISTS idx_bsv20_txos_tick_op ON bsv20_txos(tick, op, height);

CREATE INDEX IF NOT EXISTS idx_bsv20_to_validate ON bsv20_txos(height)
WHERE valid IS NULL;