CREATE TABLE IF NOT EXISTS blocks(
    hash BYTEA PRIMARY KEY,
    height INTEGER,
    processed TIMESTAMP DEFAULT current_timestamp
);