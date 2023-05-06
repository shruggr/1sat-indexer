DROP INDEX idx_inscriptions_height_idx_vout;
CREATE INDEX idx_inscriptions_height_idx_vout
ON inscriptions(height, idx, vout)
WHERE id = -1;

ALTER TABLE txos ALTER COLUMN spend SET DEFAULT decode('', 'hex');
UPDATE txos SET spend = decode('', 'hex') WHERE spend IS NULL;
ALTER TABLE txos ALTER COLUMN spend SET NOT NULL;

ALTER TABLE inscriptions ALTER COLUMN id SET DEFAULT -1;
UPDATE inscriptions SET id = -1 WHERE id IS NULL;
ALTER TABLE txos ALTER COLUMN spend SET NOT NULL;

CREATE INDEX idx_ordinal_lock_listings_price_unspent ON ordinal_lock_listings(price)
WHERE spend = decode('', 'hex');
DROP INDEX idx_ordinal_lock_listings_price;

CREATE INDEX idx_ordinal_lock_listings_num_unspent ON ordinal_lock_listings(num)
WHERE spend = decode('', 'hex');
DROP INDEX idx_ordinal_lock_listings_num;

CREATE INDEX idx_ordinal_lock_listings_height_idx_unspent ON ordinal_lock_listings(height, idx)
WHERE spend = decode('', 'hex');
DROP INDEX idx_ordinal_lock_listings_height_idx;