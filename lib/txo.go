package lib

import (
	"database/sql"
	"fmt"
)

type Txo struct {
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Satoshis uint64     `json:"satoshis"`
	AccSats  uint64     `json:"acc_sats"`
	Lock     ByteString `json:"lock"`
	Spend    ByteString `json:"spend,omitempty"`
	Origin   Origin     `json:"origin,omitempty"`
	Ordinal  uint64     `json:"ordinal"`
}

func LoadTxo(txid []byte, vout uint32) (txo *Txo, err error) {
	rows, err := GetTxo.Query(txid, vout)
	if err != nil {
		return
	}
	defer rows.Close()

	if rows.Next() {
		return bindTxo(rows)
	}
	err = &HttpError{
		StatusCode: 404,
		Err:        fmt.Errorf("not-found"),
	}
	return
}

func LoadTxos(txid []byte) (txos []*Txo, err error) {
	rows, err := GetTxos.Query(txid)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		txo, err := bindTxo(rows)
		if err != nil {
			return nil, err
		}
		txos = append(txos, txo)
	}
	return
}

func LoadUtxos(lock []byte) (utxos []*Txo, err error) {
	rows, err := GetUtxos.Query(lock)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		txo, err := bindTxo(rows)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, txo)
	}
	return
}

func bindTxo(rows *sql.Rows) (txo *Txo, err error) {
	txo = &Txo{}
	var spend *ByteString
	var origin *Origin
	err = rows.Scan(
		&txo.Txid,
		&txo.Vout,
		&txo.Satoshis,
		&txo.AccSats,
		&txo.Lock,
		&spend,
		&origin,
	)
	if spend != nil {
		txo.Spend = *spend
	}
	if origin != nil {
		txo.Origin = *origin
	}
	return
}
