package lib

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/libsv/go-bt/v2"
)

type Lock struct {
	Address string `json:"address"`
	Until   uint32 `json:"until"`
}

type TxoData struct {
	Types       []string        `json:"types,omitempty"`
	Inscription *Inscription    `json:"insc,omitempty"`
	Map         Map             `json:"map,omitempty"`
	B           *File           `json:"b,omitempty"`
	Sigmas      []*Sigma        `json:"sigma,omitempty"`
	Listing     *Listing        `json:"list,omitempty"`
	Bsv20       *Bsv20          `json:"bsv20,omitempty"`
	Claims      []*Claim        `json:"claims,omitempty"`
	Lock        *Lock           `json:"lock,omitempty"`
	OpNSMint    *OpNS           `json:"opnsMint,omitempty"`
	OpNS        *OpNS           `json:"opns,omitempty"`
	Sigil       json.RawMessage `json:"sigil,omitempty"`
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
	Data         *TxoData  `json:"data,omitempty"`
	Outpoint     *Outpoint `json:"outpoint"`
	IsOrigin     bool      `json:"-"`
	ImpliedBsv20 bool      `json:"-"`
}

func (t *Txo) Save() {
	var err error
	for i := 0; i < 3; i++ {
		_, err = Db.Exec(context.Background(), `
			INSERT INTO txos(txid, vout, outpoint, satoshis, outacc, pkhash, origin, height, idx, data)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT(outpoint) DO UPDATE SET
				satoshis=EXCLUDED.satoshis,
				outacc=EXCLUDED.outacc,
				pkhash=EXCLUDED.pkhash,
				origin=EXCLUDED.origin,
				height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				data=EXCLUDED.data`,
			t.Txid,
			t.Vout,
			t.Outpoint,
			t.Satoshis,
			t.OutAcc,
			t.PKHash,
			t.Origin,
			t.Height,
			t.Idx,
			t.Data,
		)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				log.Println(pgErr.Code, pgErr.Message)
				if pgErr.Code == "23505" {
					time.Sleep(100 * time.Millisecond)
					log.Printf("Conflict. Retrying Insert %x %d\n", t.Txid, t.Vout)
					continue
				}
			}
			log.Panicln("insTxo Err:", err)
		}
		break
	}
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}
