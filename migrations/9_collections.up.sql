CREATE INDEX IF NOT EXISTS idx_inscriptions_map ON inscriptions USING GIN(map);

ALTER TABLE inscriptions ADD COLUMN IF NOT EXISTS search_text_en TSVECTOR 
GENERATED ALWAYS AS (
	to_tsvector('english',
		COALESCE(jsonb_extract_path_text(map, 'name'), '') || ' ' || 
		COALESCE(jsonb_extract_path_text(map, 'description'), '')
	)
) STORED;

CREATE INDEX IF NOT EXISTS idx_inscriptions_search_text_en ON inscriptions USING GIN(search_text_en);

ALTER TABLE inscriptions ADD COLUMN IF NOT EXISTS sigma JSONB;
CREATE INDEX IF NOT EXISTS idx_inscriptions_sigma ON inscriptions USING GIN(sigma);

ALTER TABLE metadata ADD COLUMN IF NOT EXISTS sigma JSONB;
