package main

import (
	"context"

	"github.com/shruggr/1sat-indexer/lib"
)

type Listing struct {
	Price  uint64 `json:"price"`
	PayOut []byte `json:"payout"`
}

func SaveListing(t *lib.Txo) (err error) {
	_, err = Db.Exec(context.Background(), `
		INSERT INTO listings(txid, vout, height, idx, price, payout, origin, num, spend, pkhash, data, bsv20)
		SELECT $1, $2, $3, $4, $5, $6, t.origin, n.num, t.spend, t.pkhash, o.data,
			CASE WHEN t.data->'bsv20' IS NULL THEN false ELSE true END
		FROM txos t
		JOIN txos o ON o.outpoint = t.origin
		JOIN origins n ON n.origin = t.origin
		WHERE t.txid=$1 AND t.vout=$2
		ON CONFLICT(txid, vout) DO UPDATE SET 
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			origin=EXCLUDED.origin,
			num=EXCLUDED.num`,
		t.Txid,
		t.Vout,
		t.Height,
		t.Idx,
		t.Data.Listing.Price,
		t.Data.Listing.PayOut,
	)

	if err != nil {
		panic(err)
	}
	return
}
