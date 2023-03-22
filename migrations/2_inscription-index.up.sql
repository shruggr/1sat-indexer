CREATE INDEX idx_inscriptions_origin ON inscriptions(origin);

CREATE INDEX idx_inscriptions_id_height_idx ON inscriptions(id, height, idx);

-- CREATE INDEX idx_inscriptions_height_idx ON inscriptions(height);

-- CREATE INDEX idx_inscriptions_id
-- ON inscriptions(id);

-- CREATE INDEX idx_inscriptions_ordinal
-- ON inscriptions(ordinal);

-- CREATE INDEX idx_inscriptions_filehash
-- ON inscriptions(filehash);