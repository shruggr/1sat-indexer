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
	Idx      uint32     `json:"idx"`
	Listing  bool       `json:"listing,omitempty"`
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
	)
	if err != nil {
		log.Println("insTxo Err:", err)
		return
	}

	return
}

func (t *Txo) SaveSpend() (err error) {
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
		err = rows.Scan(&t.Lock, &t.Satoshis, &t.Listing)
	}

	return
}

// func LoadTxo(txid []byte, vout uint32) (txo *Txo, err error) {
// 	rows, err := GetTxo.Query(txid, vout)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	if rows.Next() {
// 		return bindTxo(rows)
// 	}
// 	err = &HttpError{
// 		StatusCode: 404,
// 		Err:        fmt.Errorf("not-found"),
// 	}
// 	return
// }

// func LoadTxos(txid []byte) (txos []*Txo, err error) {
// 	rows, err := GetTxos.Query(txid)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		txo, err := bindTxo(rows)
// 		if err != nil {
// 			return nil, err
// 		}
// 		txos = append(txos, txo)
// 	}
// 	return
// }

// func LoadUtxos(lock []byte) (utxos []*Txo, err error) {
// 	rows, err := GetUtxos.Query(lock)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		txo, err := bindTxo(rows)
// 		if err != nil {
// 			return nil, err
// 		}
// 		utxos = append(utxos, txo)
// 	}
// 	return
// }

// func bindTxo(rows *sql.Rows) (txo *Txo, err error) {
// 	txo = &Txo{}
// 	err = rows.Scan(
// 		&txo.Txid,
// 		&txo.Vout,
// 		&txo.Satoshis,
// 		&txo.AccSats,
// 		&txo.Lock,
// 		&txo.Spend,
// 		&txo.Origin,
// 	)
// 	return
// }
