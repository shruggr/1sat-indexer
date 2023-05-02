CREATE INDEX idx_inscriptions_origin ON inscriptions(origin);

CREATE INDEX idx_inscriptions_id
ON inscriptions(id);

CREATE INDEX idx_inscriptions_height_idx_vout
ON inscriptions(height, idx, vout)
WHERE id IS NULL;


-- CREATE INDEX idx_inscriptions_height_idx ON inscriptions(height);

-- CREATE INDEX idx_inscriptions_id
-- ON inscriptions(id);

-- CREATE INDEX idx_inscriptions_ordinal
-- ON inscriptions(ordinal);

-- CREATE INDEX idx_inscriptions_filehash
-- ON inscriptions(filehash);