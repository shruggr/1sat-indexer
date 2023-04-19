CREATE TABLE IF NOT EXISTS txn_io(
    txid BLOB,
    io BOOLEAN, -- false = input, true = output
    idx INT, -- index in inputs or outputs arrays
    sats_acc BIGINT, -- accumulated satoshis for this transaction
    ref_txid BLOB, -- for input previous txid, for outputs it is the txid which spent it, or null if unspent
    ref_idx INT, -- for input previous transaction index, for outputs spend_vin
    satoshis BIGINT, -- output satoshis
    scripthash BLOB, -- store scripts in outputs only
    block_height INT STATIC,
    block_idx INT STATIC,
    PRIMARY KEY(txid, io, idx)
);

CREATE TABLE IF NOT EXISTS block_txns(
    height INT,
    fee_acc BIGINT,
    idx INT,
    txid BLOB,
    fee BIGINT,
    PRIMARY KEY(height, fee_acc, idx)
);

CREATE TABLE IF NOT EXISTS scripthash_spent(
    scripthash BLOB,
    spent BOOLEAN,
    txid BLOB,
    idx INT,
    PRIMARY KEY(txid, idx)
);