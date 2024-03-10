package lib

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/libsv/go-bt/v2"
)

type Txo struct {
	Tx          *bt.Tx    `json:"-"`
	Outpoint    *Outpoint `json:"outpoint,omitempty"`
	Height      *uint32   `json:"height,omitempty"`
	Idx         uint64    `json:"idx"`
	Satoshis    uint64    `json:"satoshis"`
	OutAcc      uint64    `json:"outacc"`
	PKHash      []byte    `json:"pkhash"`
	Spend       []byte    `json:"spend"`
	Vin         uint32    `json:"vin"`
	SpendHeight *uint32   `json:"spend_height"`
	SpendIdx    uint64    `json:"spend_idx"`
	Origin      *Outpoint `json:"origin,omitempty"`
	Data        Map       `json:"data,omitempty"`
	Script      []byte    `json:"-"`
}

func (t *Txo) AddData(key string, value interface{}) {
	if t.Data == nil {
		t.Data = map[string]interface{}{}
	}
	t.Data[key] = value
}

func LoadTxoData(outpoint *Outpoint) (data Map, err error) {
	var dataStr sql.NullString
	err = Db.QueryRow(context.Background(), `
		SELECT data
		FROM txos
		WHERE outpoint=$1`,
		outpoint,
	).Scan(
		&dataStr,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if dataStr.Valid {
		err = json.Unmarshal([]byte(dataStr.String), &data)
		if err != nil {
			log.Panic(err)
		}
	}
	return data, nil
}

func (t *Txo) Save() {
	var err error
	for i := 0; i < 3; i++ {
		_, err = Db.Exec(context.Background(), `
			INSERT INTO txos(outpoint, satoshis, outacc, pkhash, origin, height, idx, data)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT(outpoint) DO UPDATE SET
				satoshis=EXCLUDED.satoshis,
				outacc=EXCLUDED.outacc,
				pkhash=CASE WHEN EXCLUDED.pkhash IS NULL THEN txos.pkhash ELSE EXCLUDED.pkhash END,
				origin=CASE WHEN EXCLUDED.origin IS NULL THEN txos.origin ELSE EXCLUDED.origin END,
				height=CASE WHEN EXCLUDED.height IS NULL THEN txos.height ELSE EXCLUDED.height END,
				idx=CASE WHEN EXCLUDED.height IS NULL THEN txos.idx ELSE EXCLUDED.idx END,
				data=CASE WHEN txos.data IS NULL 
					THEN EXCLUDED.data 
					ELSE CASE WHEN EXCLUDED.data IS NULL THEN txos.data ELSE txos.data || EXCLUDED.data END
				END`,
			t.Outpoint,
			t.Satoshis,
			t.OutAcc,
			t.PKHash,
			t.Origin,
			t.Height,
			t.Idx,
			t.Data,
		)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					time.Sleep(100 * time.Millisecond)
					// log.Printf("Conflict. Retrying Save %s\n", t.Outpoint)
					continue
				}
				// if pgErr.Code == "22P05" {
				// 	delete(t.Data, "insc")
				// 	continue
				// }
			}
			log.Panicf("insTxo Err: %s - %v", t.Outpoint, err)
		}
		break
	}
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}

func (t *Txo) SaveSpend() {
	var err error
	for i := 0; i < 3; i++ {
		_, err = Db.Exec(context.Background(), `
			INSERT INTO txos(outpoint, spend, vin, spend_height, spend_idx)
			VALUES($1, $2, $3, $4, $5)
			ON CONFLICT(outpoint) DO UPDATE SET
				spend=EXCLUDED.spend,
				vin=EXCLUDED.vin, 
				spend_height=CASE WHEN EXCLUDED.spend_height IS NULL THEN txos.spend_height ELSE EXCLUDED.spend_height END, 
				spend_idx=CASE WHEN EXCLUDED.spend_height IS NULL THEN txos.spend_idx ELSE EXCLUDED.spend_idx END`,
			t.Outpoint,
			t.Spend,
			t.Vin,
			t.SpendHeight,
			t.SpendIdx,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					time.Sleep(100 * time.Millisecond)
					// log.Printf("Conflict. Retrying SaveSpend %s\n", t.Outpoint)
					continue
				}
			}
			log.Panicln("insTxo Err:", err)
		}
		break
	}
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}

func (t *Txo) SetOrigin(origin *Outpoint) {
	var err error
	for i := 0; i < 3; i++ {
		_, err = Db.Exec(context.Background(), `
			INSERT INTO txos(outpoint, origin, satoshis, outacc)
			VALUES($1, $2, $3, $4)
			ON CONFLICT(outpoint) DO UPDATE SET
				origin=EXCLUDED.origin`,
			t.Outpoint,
			origin,
			t.Satoshis,
			t.OutAcc,
		)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					time.Sleep(100 * time.Millisecond)
					// log.Printf("Conflict. Retrying SetOrigin %s\n", t.Outpoint)
					continue
				}
			}
			log.Panicln("insTxo Err:", err)
		}
		break
	}
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
}
