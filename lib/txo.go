package lib

import (
	"log"
)

// type TxoFlags struct {
// 	B       bool `json:"b"`
// 	Bsv20   bool `json:"bsv20"`
// 	Listing bool `json:"listing"`
// }

type Txo struct {
	Txid       ByteString      `json:"txid"`
	Vout       uint32          `json:"vout"`
	Outpoint   *Outpoint       `json:"outpoint"`
	Height     *uint32         `json:"height"`
	Idx        *uint64         `json:"idx"`
	Satoshis   uint64          `json:"satoshis,omitempty"`
	OutAcc     uint64          `json:"outacc,omitempty"`
	Scripthash ByteString      `json:"scripthash"`
	Lock       ByteString      `json:"lock"`
	Spend      ByteString      `json:"spend,omitempty"`
	Vin        uint32          `json:"vin"`
	InAcc      uint64          `json:"inacc,omitempty"`
	Origin     *Outpoint       `json:"origin,omitempty"`
	Flags      map[string]bool `json:"flags"`
	InOrd      *Txo            `json:"inOrd"`
}

func (t *Txo) Save() {
	_, err := db.Exec(`INSERT INTO txos(txid, vout, height, idx, satoshis, outacc, scripthash, lock, flags)
		VALUES($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO UPDATE SET 
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			flags=EXCLUDED.flags`,
		t.Txid,
		t.Vout,
		t.Height,
		t.Vout,
		t.Satoshis,
		t.OutAcc,
		t.Scripthash,
		t.Lock,
		t.Flags,
	)
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}

func (t *Txo) SaveSpend() {
	row := db.QueryRow(`UPDATE txos
		SET spend=$3, inacc=$4, vin=$5
		WHERE txid=$1 AND vout=$2
		RETURNING scripthash, satoshis, flags`,
		t.Txid,
		t.Vout,
		t.Spend,
		t.InAcc,
		t.Vin,
	)

	err := row.Scan(&t.Scripthash, &t.Satoshis, &t.Flags)
	if err != nil {
		log.Panic(err)
	}

}
