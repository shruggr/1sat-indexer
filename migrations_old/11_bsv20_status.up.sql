CREATE INDEX CONCURRENTLY idx_bsv20_txos_pkhash_id_status_height_idx ON bsv20_txos(pkhash, id, status, height, idx)
WHERE id != '\x'::bytea;

DROP INDEX idx_bsv20_txos_pkhash_id_status;

CREATE INDEX CONCURRENTLY idx_bsv20_txos_pkhash_tick_status_height_idx ON bsv20_txos(pkhash, tick, status, height, idx)
WHERE tick != '';