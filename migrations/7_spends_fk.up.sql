ALTER TABLE ordinal_lock_listings ADD COLUMN spend BYTEA;
CREATE UNIQUE INDEX idx_txos_txid_vout_spend ON txos(txid, vout, spend);

UPDATE ordinal_lock_listings l
SET spend = (
    SELECT spend 
    FROM txos
    WHERE txid = l.txid AND vout = l.vout
);

ALTER TABLE ordinal_lock_listings 
    ADD CONSTRAINT fk_ordinal_lock_listings_spend 
    FOREIGN KEY (txid, vout, spend) 
    REFERENCES txos (txid, vout, spend)
    ON UPDATE CASCADE;


CREATE UNIQUE INDEX idx_inscriptions_origin_id ON inscriptions(origin, id);
DROP INDEX idx_inscriptions_origin;
ALTER TABLE ordinal_lock_listings ADD COLUMN num BIGINT;

UPDATE ordinal_lock_listings l
SET num = (
    SELECT id 
    FROM inscriptions
    WHERE txid = l.txid AND vout = l.vout
);

ALTER TABLE ordinal_lock_listings 
    ADD CONSTRAINT fk_ordinal_lock_listings_origin_num 
    FOREIGN KEY (origin, num) 
    REFERENCES inscriptions(origin, id)
    ON UPDATE CASCADE;

CREATE INDEX idx_ordinal_lock_listings_price ON ordinal_lock_listings(price)
WHERE spend IS NULL;

CREATE INDEX idx_ordinal_lock_listings_num ON ordinal_lock_listings(num)
WHERE spend IS NULL;

CREATE INDEX idx_ordinal_lock_listings_height_idx ON ordinal_lock_listings(height, idx)
WHERE spend IS NULL;