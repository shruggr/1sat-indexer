CREATE TABLE claims(
    txid BYTEA,
    vout NUMBER,
    height INTEGER,
    idx BIGINT,
    origin BYTEA,
    sub TEXT,
    type TEXT,
    target BYTEA,
    PRIMARY KEY(txid, vout, sub, type)
);
CREATE INDEX idx_claims_origin_sub_type ON claims(origin, sub, type, height DESC, idx DESC);
CREATE INDEX idx_claims_target ON claims(target, height DESC, idx DESC);