ALTER TABLE txos ADD COLUMN bsv20_xfer INTEGER GENERATED ALWAYS AS (
    CASE WHEN data->'bsv20'->>'op' = 'transfer' 
    THEN CAST(data->'bsv20'->>'status' as INTEGER)
    ELSE NULL
    END
) STORED;

CREATE INDEX idx_bsv20_transfers_height_idx ON txos(status, height, idx)
WHERE bsv20_xfer IS NOT NULL;

CREATE TABLE IF NOT EXISTS bsv20 (
    txid BYTEA,
    vout INT,
    height INT,
    idx BIGINT,
    tick TEXT,
    max NUMERIC NOT NULL,
    lim NUMERIC NOT NULL,
    dec INT DEFAULT 18,
    supply NUMERIC DEFAULT 0,
    status INT DEFAULT 0,
    available NUMERIC GENERATED ALWAYS AS (max - supply) STORED,
    pct_minted NUMERIC GENERATED ALWAYS AS (CASE WHEN max = 0 THEN 0 ELSE ROUND(100.0 * supply / max, 1) END) STORED,
    reason TEXT,
    PRIMARY KEY(txid, vout)
);
CREATE INDEX IF NOT EXISTS idx_bsv20_tick ON bsv20(tick);
CREATE INDEX IF NOT EXISTS idx_bsv20_available ON bsv20(available);
CREATE INDEX IF NOT EXISTS idx_bsv20_pct_minted ON bsv20(pct_minted);
CREATE INDEX IF NOT EXISTS idx_bsv20_max ON bsv20(max);
CREATE INDEX IF NOT EXISTS idx_bsv20_height_idx ON bsv20(height, idx, vout);
CREATE INDEX IF NOT EXISTS idx_bsv20_to_validate ON bsv20(height, idx, vout)
    WHERE status = 0;

CREATE TABLE IF NOT EXISTS bsv20_mints (
    txid BYTEA,
    vout INT,
    height INT,
    idx BIGINT,
    tick TEXT,
    amt NUMERIC NOT NULL,
    status INT DEFAULT 0,
    reason TEXT,
    PRIMARY KEY(txid, vout)
);

CREATE INDEX IF NOT EXISTS idx_bsv20_mints_status ON bsv20_mints(status, height, idx, vout);

CREATE INDEX IF NOT EXISTS idx_bsv20_mints_tick_validate ON bsv20_mints(tick, height, idx, vout)
    WHERE status = 0;