package lib

import (
	"database/sql"
	"log"
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

var getTxo *sql.Stmt
var getTxos *sql.Stmt

func init() {
	var err error
	getTxo, err = Db.Prepare(`SELECT txid, vout, satoshis, acc_sats, lock, spend, origin
		FROM txos
		WHERE txid=$1 AND vout=$2 AND acc_sats IS NOT NULL
	`)
	if err != nil {
		log.Fatal(err)
	}

	getTxos, err = Db.Prepare(`SELECT txid, vout, satoshis, acc_sats, lock, spend, origin
		FROM txos
		WHERE txid=$1 AND satoshis=1 AND acc_sats IS NOT NULL
	`)
	if err != nil {
		log.Fatal(err)
	}
}

func LoadTxo(txid []byte, vout uint32) (txo *Txo, err error) {
	txo = &Txo{}
	err = getTxo.QueryRow(txid, vout).Scan(
		&txo.Txid,
		&txo.Vout,
		&txo.Satoshis,
		&txo.AccSats,
		&txo.Lock,
		&txo.Spend,
		&txo.Origin,
	)
	if err != nil {
		return nil, err
	}
	return
}

func LoadTxos(txid []byte) (txos []*Txo, err error) {
	rows, err := getTxos.Query(txid)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		txo := &Txo{}
		err = rows.Scan(
			&txo.Txid,
			&txo.Vout,
			&txo.Satoshis,
			&txo.AccSats,
			&txo.Lock,
			&txo.Spend,
			&txo.Origin,
		)
		if err != nil {
			return
		}
		txos = append(txos, txo)
	}
	return
}
