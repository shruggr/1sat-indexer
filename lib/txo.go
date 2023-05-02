package lib

import (
	"log"
)

type Txo struct {
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Satoshis uint64     `json:"satoshis,omitempty"`
	OutAcc   uint64     `json:"outacc,omitempty"`
	Lock     []byte     `json:"lock"`
	Spend    ByteString `json:"spend,omitempty"`
	// Vin      uint32     `json:"vin"`
	InAcc   uint64    `json:"inacc,omitempty"`
	Origin  *Outpoint `json:"origin,omitempty"`
	Ordinal uint64    `json:"ordinal"`
	// Height   uint32     `json:"height"`
	// Idx      uint32     `json:"idx"`
	// Listing  bool       `json:"listing,omitempty"`
}

func (t *Txo) Save() (err error) {
	_, err = InsTxo.Exec(
		t.Txid,
		t.Vout,
		t.Satoshis,
		t.OutAcc,
		t.Lock,
		t.Ordinal,
		// t.Height,
		// t.Idx,
	)
	if err != nil {
		log.Println("insTxo Err:", err)
		return
	}

	return
}

// func (t *Txo) SaveWithSpend() (err error) {
// 	_, err = InsSpend.Exec(
// 		t.Txid,
// 		t.Vout,
// 		t.Satoshis,
// 		t.OutAcc,
// 		t.Lock,
// 		t.Origin,
// 		t.Height,
// 		t.Idx,
// 		t.Spend,
// 	)
// 	if err != nil {
// 		log.Println("insTxo Err:", err)
// 		return
// 	}

// 	return
// }

func (t *Txo) SaveSpend() (update bool, err error) {
	rows, err := SetSpend.Query(
		t.Txid,
		t.Vout,
		t.Spend,
		t.InAcc,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	if rows.Next() {
		err = rows.Scan(&t.Lock, &t.Satoshis, &t.Ordinal)
		if err != nil {
			return
		}
		update = true
	}

	return
}
