CREATE INDEX CONCURRENTLY idx_txos_data ON txos USING GIN(data)
    WHERE data IS NOT NULL;

ALTER TABLE txos ADD COLUMN search_text_en TSVECTOR GENERATED ALWAYS AS (
    to_tsvector('english',
        COALESCE(jsonb_extract_path_text(data, 'insc.text'), '') || ' ' || 
        COALESCE(jsonb_extract_path_text(data, 'map.name'), '') || ' ' || 
        COALESCE(jsonb_extract_path_text(data, 'map.description'), '') || ' ' || 
        COALESCE(jsonb_extract_path_text(data, 'map.subTypeData.description'), '') || ' ' || 
        COALESCE(jsonb_extract_path_text(data, 'map.keywords'), '')
    )
) STORED;

ALTER TABLE txos ADD COLUMN geohash TEXT GENERATED ALWAYS AS (
    jsonb_extract_path_text(data, 'map.geohash')
) STORED;


CREATE INDEX CONCURRENTLY idx_txos_search_text_en ON txos USING GIN(search_text_en);
CREATE INDEX CONCURRENTLY idx_txos_geohash ON txos(geohash text_pattern_ops)
    WHERE geohash IS NOT NULL;

-- DROP INDEX idx_origins_data;
-- ALTER TABLE origins DROP COLUMN data;
-- ALTER TABLE origins ADD COLUMN map JSONB;
-- CREATE INDEX idx_origins_map ON origins USING GIN(map)
--     WHERE map IS NOT NULL;
