package lib

import (
	"context"
	"log"
)

type Txo struct {
	Txid     []byte    `json:"txid"`
	Vout     uint32    `json:"vout"`
	Height   *uint32   `json:"height"`
	Idx      uint64    `json:"idx"`
	Satoshis uint64    `json:"satoshis"`
	OutAcc   uint64    `json:"outacc"`
	PKHash   []byte    `json:"pkhash"`
	Spend    *Spend    `json:"spend"`
	Vin      uint32    `json:"vin"`
	Origin   *Outpoint `json:"origin"`
	Listing  bool      `json:"listing"`
	Bsv20    bool      `json:"bsv20"`
	Outpoint *Outpoint `json:"outpoint"`
	IsOrigin bool
}

func (t *Txo) Save() {
	_, err := Db.Exec(context.Background(), `
		INSERT INTO txos(txid, vout, satoshis, outacc, pkhash, origin, height, idx, listing, bsv20)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT(txid, vout) DO UPDATE SET
			satoshis=EXCLUDED.satoshis,
			outacc=EXCLUDED.outacc,
			pkhash=EXCLUDED.pkhash,
			origin=EXCLUDED.origin,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			listing=EXCLUDED.listing,
			bsv20=EXCLUDED.bsv20`,
		t.Txid,
		t.Vout,
		t.Satoshis,
		t.OutAcc,
		t.PKHash,
		t.Origin,
		t.Height,
		t.Idx,
		t.Listing,
		t.Bsv20,
	)
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}
