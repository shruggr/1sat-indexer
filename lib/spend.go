package lib

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
)

type Spend struct {
	Txid     []byte    `json:"txid"`
	Vout     uint32    `json:"vout"`
	Satoshis uint64    `json:"satoshis,omitempty"`
	OutAcc   uint64    `json:"outacc"`
	InAcc    uint64    `json:"inacc"`
	PKHash   []byte    `json:"pkhash"`
	Spend    []byte    `json:"spend,omitempty"`
	Vin      uint32    `json:"vin"`
	Height   *uint32   `json:"height"`
	Idx      uint64    `json:"idx"`
	Origin   *Outpoint `json:"origin,omitempty"`
	Data     *TxoData  `json:"data,omitempty"`
	Outpoint *Outpoint `json:"outpoint,omitempty"`
	Missing  bool      `json:"missing"`
}

func (s *Spend) SetSpent() (exists bool) {
	rows, err := Db.Query(context.Background(), `
		UPDATE txos
		SET spend=$3, vin=$4, spend_height=$5, spend_idx=$6
		WHERE txid=$1 AND vout=$2
		RETURNING COALESCE(pkhash, '\x'), satoshis, data, origin`,
		s.Txid,
		s.Vout,
		s.Spend,
		s.Vin,
		s.Height,
		s.Idx,
	)
	if err != nil {
		log.Panicln("setSpend Err:", err)
	}
	defer rows.Close()
	var data sql.NullString
	if rows.Next() {
		err = rows.Scan(&s.PKHash, &s.Satoshis, &data, &s.Origin)
		if err != nil {
			log.Panic(err)
		}
		if data.Valid {
			s.Data = &TxoData{}
			err = json.Unmarshal([]byte(data.String), s.Data)
			if err != nil {
				log.Panic(err)
			}
		}
		exists = true
	}

	return
}

func (s *Spend) Save() {
	if _, err := Db.Exec(context.Background(), `
		INSERT INTO txos(txid, vout, outpoint, satoshis, outacc, spend, vin, spend_height, spend_idx)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(outpoint) DO NOTHING`,
		s.Txid,
		s.Vout,
		s.Outpoint,
		s.Satoshis,
		s.OutAcc,
		s.Spend,
		s.Vin,
		s.Height,
		s.Idx,
	); err != nil {
		log.Panicf("%x: %v\n", s.Txid, err)
	}
}
