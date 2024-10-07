CREATE INDEX CONCURRENTLY idx_txns_created_unmined2 ON txns(created)
    WHERE height IS NULL OR height=0 OR idx=0;

DROP INDEX idx_txns_created_unmined;