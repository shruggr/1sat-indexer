ALTER TABLE ordinal_lock_listings ADD COLUMN IF NOT EXISTS bsv20 BOOLEAN DEFAULT FALSE;
UPDATE ordinal_lock_listings l
SET bsv20 = TRUE 
FROM bsv20_txos b
WHERE b.txid=l.txid AND b.vout=l.vout;

ALTER TABLE ordinal_lock_listings ADD COLUMN IF NOT EXISTS lock BYTEA;
UPDATE ordinal_lock_listings l
SET lock = t.lock
FROM txos t
WHERE t.txid=l.txid AND t.vout=l.vout;

CREATE INDEX idx_ordinal_lock_listings_bsv20_price_unspent ON ordinal_lock_listings(bsv20, price)
WHERE spend = decode('', 'hex');
DROP INDEX IF EXISTS idx_ordinal_lock_listings_price_unspent;

CREATE INDEX idx_ordinal_lock_listings_bsv20_num_unspent ON ordinal_lock_listings(bsv20, num)
WHERE spend = decode('', 'hex');
DROP INDEX IF EXISTS idx_ordinal_lock_listings_num_unspent;

CREATE INDEX idx_ordinal_lock_listings_bsv20_height_idx_unspent ON ordinal_lock_listings(bsv20, height, idx)
WHERE spend = decode('', 'hex');
DROP INDEX IF EXISTS idx_ordinal_lock_listings_height_idx_unspent;

ALTER TABLE bsv20_txos ADD COLUMN IF NOT EXISTS listing BOOLEAN DEFAULT FALSE;
UPDATE bsv20_txos b
SET listing = TRUE 
FROM ordinal_lock_listings l
WHERE l.txid=b.txid AND l.vout=b.vout;

CREATE INDEX IF NOT EXISTS idx_bsv20_available ON bsv20(available);
CREATE INDEX IF NOT EXISTS idx_bsv20_pct_minted ON bsv20(pct_minted);
CREATE INDEX IF NOT EXISTS idx_bsv20_max ON bsv20(max);
CREATE INDEX IF NOT EXISTS idx_bsv20_height_idx ON bsv20(height, idx);

CREATE INDEX IF NOT EXISTS idx_bsv20_txos_lock_spend_tick_height_idx ON bsv20_txos(lock, spend, tick, height, idx);

ALTER TABLE bsv20_txos ADD COLUMN orig_amt NUMERIC;
ALTER TABLE bsv20_txos ALTER COLUMN amt TYPE NUMERIC;

ALTER TABLE bsv20 ALTER COLUMN max TYPE NUMERIC;
ALTER TABLE bsv20 ALTER COLUMN lim TYPE NUMERIC;
ALTER TABLE bsv20 ALTER COLUMN supply TYPE NUMERIC;

ALTER TABLE bsv20 ADD COLUMN IF NOT EXISTS available NUMERIC 
GENERATED ALWAYS AS (
	max - supply
) STORED;

ALTER TABLE bsv20 ADD COLUMN IF NOT EXISTS pct_minted NUMERIC
GENERATED ALWAYS AS (
	CASE WHEN max = 0 THEN 0 ELSE ROUND(100.0 * supply / max, 1) END
) STORED;
