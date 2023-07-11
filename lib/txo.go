package lib

import (
	"context"
	"log"

	"github.com/libsv/go-bt/v2"
)

type TxoData struct {
	Types       []string     `json:"types"`
	Inscription *Inscription `json:"inscription"`
	Map         Map          `json:"map"`
	B           *File        `json:"b"`
	Sigmas      []*Sigma     `json:"sigma"`
	Listing     *Listing     `json:"listing"`
	Bsv20       *Bsv20       `json:"bsv20"`
}

type Txo struct {
	Tx           *bt.Tx    `json:"-"`
	Txid         []byte    `json:"txid"`
	Vout         uint32    `json:"vout"`
	Height       *uint32   `json:"height"`
	Idx          uint64    `json:"idx"`
	Satoshis     uint64    `json:"satoshis"`
	OutAcc       uint64    `json:"outacc"`
	PKHash       []byte    `json:"pkhash"`
	Spend        *Spend    `json:"spend"`
	Vin          uint32    `json:"vin"`
	Origin       *Outpoint `json:"origin"`
	Data         *TxoData  `json:"data"`
	Outpoint     *Outpoint `json:"outpoint"`
	IsOrigin     bool
	ImpliedBsv20 bool
}

func (t *Txo) Save() {
	_, err := Db.Exec(context.Background(), `
		INSERT INTO txos(txid, vout, satoshis, outacc, pkhash, origin, height, idx, data)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT(txid, vout) DO UPDATE SET
			satoshis=EXCLUDED.satoshis,
			outacc=EXCLUDED.outacc,
			pkhash=EXCLUDED.pkhash,
			origin=EXCLUDED.origin,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			data=EXCLUDED.data`,
		t.Txid,
		t.Vout,
		t.Satoshis,
		t.OutAcc,
		t.PKHash,
		t.Origin,
		t.Height,
		t.Idx,
		t.Data,
	)
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}
