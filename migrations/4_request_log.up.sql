CREATE TABLE request_log(
    seq BIGSERIAL PRIMARY KEY,
    method TEXT,
    path TEXT,
    apikey TEXT,
    status INTEGER,
    error TEXT,
    created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    duration NUMERIC
);

CREATE INDEX CONCURRENTLY idx_request_log_duration ON request_log(duration)