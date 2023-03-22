package lib

import (
	"database/sql"
	"fmt"
)

type Txo struct {
	Txid     []byte
	Vout     uint32
	Satoshis uint64
	AccSats  uint64
	Lock     []byte
	Spend    []byte
	Origin   []byte
	Ordinal  uint64
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

func LoadUtxos(scripthash []byte) (txos []*Txo, err error) {
	rows, err := GetUtxos.Query(scripthash)
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

func bindTxo(rows *sql.Rows) (txo *Txo, err error) {
	txo = &Txo{}
	err = rows.Scan(
		&txo.Txid,
		&txo.Vout,
		&txo.Satoshis,
		&txo.AccSats,
		&txo.Lock,
		&txo.Spend,
		&txo.Origin,
	)
	return
}
