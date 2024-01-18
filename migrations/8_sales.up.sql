ALTER TABLE listings ADD COLUMN IF NOT EXISTS sale BOOLEAN;
ALTER TABLE listings ADD COLUMN IF NOT EXISTS spend_height INTEGER;
ALTER TABLE listings ADD COLUMN IF NOT EXISTS spend_idx BIGINT;
CREATE INDEX IF NOT EXISTS idx_listings_spend ON listings(spend)
WHERE spend != '\x';
CREATE INDEX IF NOT EXISTS idx_listings_sales ON listings(sale, spend_height, spend_idx)
WHERE spend != '\x';


ALTER TABLE bsv20_txos ADD COLUMN IF NOT EXISTS sale BOOLEAN;
ALTER TABLE bsv20_txos ADD COLUMN IF NOT EXISTS spend_height INTEGER;
ALTER TABLE bsv20_txos ADD COLUMN IF NOT EXISTS spend_idx BIGINT;
CREATE INDEX IF NOT EXISTS idx_bsv20_txos_spend ON bsv20_txos(spend)
WHERE spend != '\x';
CREATE INDEX IF NOT EXISTS idx_bsv20_txos_sales ON bsv20_txos(sale, spend_height, spend_idx)
WHERE spend != '\x';
