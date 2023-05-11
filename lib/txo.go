package lib

import (
	"encoding/binary"
	"log"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

type Txo struct {
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Satoshis uint64     `json:"satoshis,omitempty"`
	OutAcc   uint64     `json:"outacc,omitempty"`
	Lock     []byte     `json:"lock"`
	Spend    ByteString `json:"spend,omitempty"`
	Vin      uint32     `json:"vin"`
	InAcc    uint64     `json:"inacc,omitempty"`
	Origin   *Outpoint  `json:"origin,omitempty"`
	Ordinal  uint64     `json:"ordinal"`
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
		// t.Ordinal,
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
		err = rows.Scan(&t.Lock, &t.Satoshis)
		if err != nil {
			return
		}
		update = true
	}

	return
}

func (t *Txo) SpendKey() fdb.Key {
	return TxnDir.Pack(tuple.Tuple{t.Txid, "i", t.InAcc})
}

func (t *Txo) SpendData() (data []byte) {
	data = append(data, t.Spend...)
	data = binary.AppendUvarint(data, uint64(t.Vout))
	data = binary.AppendUvarint(data, t.OutAcc)
	data = binary.AppendUvarint(data, t.Satoshis)
	return
}

func (t *Txo) OutputKey() fdb.Key {
	return TxnDir.Pack(tuple.Tuple{t.Txid, "o", t.Vout})
}

func (t *Txo) OutputData() (data []byte) {
	data = binary.AppendUvarint(data, t.Satoshis)
	data = binary.AppendUvarint(data, t.OutAcc)
	return
}

func (t *Txo) PopulateOutputData(data []byte) {
	t.Satoshis, _ = binary.Uvarint(data)
	t.OutAcc, _ = binary.Uvarint(data)
}
