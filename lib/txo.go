package lib

import (
	"log"
)

type Txo struct {
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Satoshis uint64     `json:"satoshis,omitempty"`
	AccSats  uint64     `json:"acc_sats,omitempty"`
	Lock     ByteString `json:"lock"`
	Spend    ByteString `json:"spend,omitempty"`
	Vin      uint32     `json:"vin"`
	Origin   *Outpoint  `json:"origin,omitempty"`
	Ordinal  uint64     `json:"ordinal"`
	Height   uint32     `json:"height"`
	Idx      uint64     `json:"idx"`
	Listing  bool       `json:"listing,omitempty"`
	Bsv20    bool       `json:"bsv20,omitempty"`
	PrevOrd  *Txo       `json:"prevOrd,omitempty"`
	Outpoint *Outpoint  `json:"outpoint,omitempty"`
}

func (t *Txo) Save() {
	_, err := InsTxo.Exec(
		t.Txid,
		t.Vout,
		t.Satoshis,
		t.AccSats,
		t.Lock,
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

func (t *Txo) SaveWithSpend() {
	_, err := InsSpend.Exec(
		t.Txid,
		t.Vout,
		t.Satoshis,
		t.AccSats,
		t.Lock,
		t.Origin,
		t.Height,
		t.Idx,
		t.Spend,
	)
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}

func (t *Txo) SaveSpend() (update bool) {
	rows, err := SetSpend.Query(
		t.Txid,
		t.Vout,
		t.Spend,
		t.Vin,
	)
	if err != nil {
		log.Panicln("setSpend Err:", err)
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&t.Lock, &t.Satoshis, &t.Listing, &t.Bsv20, &t.Origin)
		if err != nil {
			log.Panic(err)
		}
		update = true
	}

	return
}
