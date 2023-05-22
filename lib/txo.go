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
}

func (t *Txo) Save() (err error) {
	_, err = InsTxo.Exec(
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
		log.Println("insTxo Err:", err)
		return
	}

	return
}

func (t *Txo) SaveWithSpend() (err error) {
	_, err = InsSpend.Exec(
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
		log.Println("insTxo Err:", err)
		return
	}

	return
}

func (t *Txo) SaveSpend() (update bool, err error) {
	rows, err := SetSpend.Query(
		t.Txid,
		t.Vout,
		t.Spend,
		t.Vin,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&t.Lock, &t.Satoshis, &t.Listing, &t.Bsv20, &t.Origin)
		if err != nil {
			return
		}
		update = true
	}

	return
}
