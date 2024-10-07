package lib

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/bitcoin-sv/go-sdk/chainhash"
	"github.com/jackc/pgx/v5/pgconn"
)

type Txo struct {
	// Tx          *transaction.Transaction `json:"-"`
	Outpoint    *Outpoint             `json:"outpoint,omitempty"`
	Height      uint32                `json:"height,omitempty"`
	Idx         uint64                `json:"idx"`
	Satoshis    uint64                `json:"satoshis"`
	OutAcc      uint64                `json:"outacc"`
	Owners      map[string]struct{}   `json:"-"`
	Spend       *chainhash.Hash       `json:"spend"`
	SpendHeight *uint32               `json:"spend_height"`
	SpendIdx    uint64                `json:"spend_idx"`
	Origin      *Outpoint             `json:"origin,omitempty"`
	Data        map[string]*IndexData `json:"data,omitempty"`
	// Script      []byte                   `json:"-"`
}

// func (t *Txo) AddData(key string, value interface{}) {
// 	if t.Data == nil {
// 		t.Data = map[string]interface{}{}
// 	}
// 	t.Data[key] = value
// }

// func LoadTxoData(outpoint *Outpoint) (data Map, err error) {
// 	var dataStr sql.NullString
// 	err = Db.QueryRow(context.Background(), `
// 		SELECT data
// 		FROM txos
// 		WHERE outpoint=$1`,
// 		outpoint,
// 	).Scan(
// 		&dataStr,
// 	)
// 	if err != nil {
// 		if err == pgx.ErrNoRows {
// 			return nil, nil
// 		}
// 		return nil, err
// 	}
// 	if dataStr.Valid {
// 		err = json.Unmarshal([]byte(dataStr.String), &data)
// 		if err != nil {
// 			log.Panic(err)
// 		}
// 	}
// 	return data, nil
// }

func (t *Txo) Save() {
	var err error
	owners := make([]string, 0, len(t.Owners))
	for owner := range t.Owners {
		owners = append(owners, owner)
	}
	for i := 0; i < 3; i++ {
		_, err = Db.Exec(context.Background(), `
			INSERT INTO txos(outpoint, satoshis, outacc, height, idx)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT(outpoint) DO UPDATE SET
				satoshis=EXCLUDED.satoshis,
				outacc=EXCLUDED.outacc,
				origin=CASE WHEN EXCLUDED.origin IS NULL THEN txos.origin ELSE EXCLUDED.origin END,
				height=CASE WHEN EXCLUDED.height IS NULL THEN txos.height ELSE EXCLUDED.height END,
				idx=CASE WHEN EXCLUDED.height IS NULL THEN txos.idx ELSE EXCLUDED.idx END`,
			t.Outpoint,
			t.Satoshis,
			t.OutAcc,
			owners,
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
			out, err2 := json.Marshal(t.Data)
			log.Panicf("insTxo Err: %s - %v %s %v", t.Outpoint, err, out, err2)
		}
		break
	}
	if err != nil {
		out, err2 := json.Marshal(t.Data)
		log.Panicln("insTxo Err:", err, string(out), err2)
	}
}

func (t *Txo) SaveSpend(ctx context.Context) {
	var err error
	for i := 0; i < 3; i++ {
		_, err = Db.Exec(ctx, `
			INSERT INTO txos(outpoint, spend, spend_height, spend_idx)
			VALUES($1, $2, $3, $4)
			ON CONFLICT(outpoint) DO UPDATE SET
				spend=EXCLUDED.spend,
				spend_height=CASE WHEN EXCLUDED.spend_height > 50000000 THEN txos.spend_height ELSE EXCLUDED.spend_height END,
				spend_idx=CASE WHEN EXCLUDED.spend_height > 50000000 THEN txos.spend_idx ELSE EXCLUDED.spend_idx END`,
			t.Outpoint,
			t.Spend,
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

// func (t *Txo) SetOrigin(origin *Outpoint) {
// 	var err error
// 	for i := 0; i < 3; i++ {
// 		_, err = Db.Exec(context.Background(), `
// 			INSERT INTO txos(outpoint, origin, satoshis, outacc)
// 			VALUES($1, $2, $3, $4)
// 			ON CONFLICT(outpoint) DO UPDATE SET
// 				origin=EXCLUDED.origin`,
// 			t.Outpoint,
// 			origin,
// 			t.Satoshis,
// 			t.OutAcc,
// 		)

// 		if err != nil {
// 			var pgErr *pgconn.PgError
// 			if errors.As(err, &pgErr) {
// 				if pgErr.Code == "23505" {
// 					time.Sleep(100 * time.Millisecond)
// 					// log.Printf("Conflict. Retrying SetOrigin %s\n", t.Outpoint)
// 					continue
// 				}
// 			}
// 			log.Panicln("insTxo Err:", err)
// 		}
// 		break
// 	}
// 	if err != nil {
// 		log.Panicln("insTxo Err:", err)
// 	}
// }
