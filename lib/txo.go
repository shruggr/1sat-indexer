package lib

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
)

type Txo struct {
	Txid       []byte `json:"txid"`
	Vout       uint32 `json:"vout"`
	Satoshis   uint64 `json:"satoshis"`
	AccSatsOut uint64 `json:"accSatsOut"`
	AccSatsIn  uint64 `json:"accSatsIn"`
	ScriptHash []byte `json:"scripthash"`
	Lock       []byte `json:"lock"`
	SpendTxid  []byte `json:"spend,omitempty"`
	SpendVin   uint32 `json:"vin,omitempty"`
}

func (t *Txo) OutputKey() []byte {
	buf := bytes.NewBuffer([]byte{'t'})
	binary.Write(buf, binary.BigEndian, t.Txid[:32])
	binary.Write(buf, binary.BigEndian, 'o')
	binary.Write(buf, binary.BigEndian, t.Vout)
	return buf.Bytes()
}

func (t *Txo) OutputValue() []byte {
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, t.Satoshis)
	binary.Write(buf, binary.BigEndian, t.AccSatsOut)
	binary.Write(buf, binary.BigEndian, t.SpendTxid)
	binary.Write(buf, binary.BigEndian, t.SpendVin)
	binary.Write(buf, binary.BigEndian, t.ScriptHash[:32])
	return buf.Bytes()
}

func (t *Txo) OutputPopulate(data []byte) (err error) {
	buf := bytes.NewBuffer(data)
	err = binary.Read(buf, binary.BigEndian, &t.Satoshis)
	if err != nil {
		return
	}
	err = binary.Read(buf, binary.BigEndian, &t.AccSatsOut)
	if err != nil {
		return
	}
	err = binary.Read(buf, binary.BigEndian, &t.SpendTxid)
	if err != nil {
		return
	}
	err = binary.Read(buf, binary.BigEndian, &t.SpendVin)
	if err != nil {
		return
	}
	t.ScriptHash = make([]byte, 32)
	err = binary.Read(buf, binary.BigEndian, &t.ScriptHash)
	if err != nil {
		return
	}
	return

}

func (t *Txo) InputKey() []byte {
	buf := bytes.NewBuffer([]byte{'t'})
	binary.Write(buf, binary.BigEndian, t.SpendTxid)
	binary.Write(buf, binary.BigEndian, 'i')
	binary.Write(buf, binary.BigEndian, t.AccSatsIn)
	return buf.Bytes()
}

func (t *Txo) InputValue() []byte {
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, t.SpendVin)
	binary.Write(buf, binary.BigEndian, t.Txid)
	binary.Write(buf, binary.BigEndian, t.Vout)
	binary.Write(buf, binary.BigEndian, t.Satoshis)
	return buf.Bytes()
}

func (t *Txo) ScriptHashKey() []byte {
	buf := bytes.NewBuffer([]byte{'s'})
	binary.Write(buf, binary.BigEndian, t.ScriptHash[:32])
	binary.Write(buf, binary.BigEndian, len(t.SpendTxid) > 0)
	binary.Write(buf, binary.BigEndian, t.Txid[:32])
	binary.Write(buf, binary.BigEndian, t.Vout)
	return buf.Bytes()
}

func (t *Txo) ByScriptHashKey() []byte {
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, t.Satoshis)
	return buf.Bytes()
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
	err = rows.Scan(
		&txo.Txid,
		&txo.Vout,
		&txo.Satoshis,
		&txo.AccSats,
		&txo.Lock,
		&txo.SpendTxid,
		&txo.Origin,
	)
	return
}
