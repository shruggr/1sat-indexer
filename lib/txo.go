package lib

import (
	"bytes"
	"context"
	"errors"
	"log"
	"time"

	"github.com/bitcoin-sv/go-sdk/util"
	"github.com/jackc/pgx/v5"
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
	Spend       ByteString            `json:"spend"`
	SpendHeight uint32                `json:"spend_height"`
	SpendIdx    uint64                `json:"spend_idx"`
	Origin      *Outpoint             `json:"origin,omitempty"`
	Data        map[string]*IndexData `json:"data,omitempty"`
	// Script      []byte                   `json:"-"`
}

func (t *Txo) EnsureTxn(ctx context.Context) error {
	txid := t.Outpoint.Txid()
	log.Printf("EnsureTxn %x\n", txid)
	row := Db.QueryRow(ctx,
		`SELECT true FROM txns WHERE txid=$1`,
		txid,
	)
	var exists bool
	var height uint32
	var idx uint64
	if err := row.Scan(&exists); err == pgx.ErrNoRows {
		if tx, err := LoadTx(ctx, t.Outpoint.TxidHex()); err != nil {
			log.Panicln(err)
			return err
		} else if tx.MerklePath != nil {
			height = tx.MerklePath.BlockHeight
			leTxid := util.ReverseBytes(txid)
			for _, path := range tx.MerklePath.Path[0] {
				if bytes.Equal((*path.Hash)[:32], leTxid) {
					idx = path.Offset
					break
				}
			}
			t.Satoshis = tx.Outputs[t.Outpoint.Vout()].Satoshis
		} else {
			height = uint32(time.Now().Unix())
			t.Satoshis = tx.Outputs[t.Outpoint.Vout()].Satoshis
		}

		for i := 0; i < 3; i++ {
			if _, err := Db.Exec(ctx, `INSERT INTO txns(txid, height, idx)
				VALUES($1, $2, $3)
				ON CONFLICT(txid) DO NOTHING`,
				txid,
				height,
				idx,
			); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					if pgErr.Code == "23505" {
						time.Sleep(100 * time.Millisecond)
						log.Printf("Conflict. Retrying SaveSpend %s\n", t.Outpoint)
						continue
					}
				}
				log.Panicln("saveSpend Err:", err)
			}
			break
		}
	} else if err != nil {
		log.Panicln(err)
		return err
	}
	return nil
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

func (t *Txo) Save() error {
	var err error
	log.Printf("SaveTxo %s %s\n", t.Outpoint, t.Spend)
	for i := 0; i < 3; i++ {
		if _, err = Db.Exec(context.Background(), `
			INSERT INTO txos(outpoint, txid, vout, satoshis, outacc, height, idx)
			SELECT $1, txid, $3, $4, $5, height, idx
			FROM txns
			WHERE txid=$2
			ON CONFLICT(outpoint) DO UPDATE SET
				satoshis=EXCLUDED.satoshis,
				outacc=EXCLUDED.outacc,
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			*t.Outpoint,
			t.Outpoint.Txid(),
			t.Outpoint.Vout(),
			t.Satoshis,
			t.OutAcc,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					time.Sleep(100 * time.Millisecond)
					log.Printf("Conflict. Retrying Save Txo %s\n", t.Outpoint)
					continue
				}
			}
			log.Panicln("insTxo Err:", err)
			return err
		}
		break
	}
	if err != nil {
		log.Panicln("insTxo Err:", err)
		return err
	}
	var owners []string
	for owner := range t.Owners {
		owners = append(owners, owner)
	}
	if _, err = Db.Exec(context.Background(), `
		INSERT INTO owners(owner, txid, vout, height, idx, spend, spend_height, spend_idx)
		SELECT o.owner, t.txid, t.vout, t.height, t.idx, t.spend, t.spend_height, t.spend_idx
		FROM txos t, unnest($1::text[]) o(owner)
		WHERE t.outpoint=$2
		ON CONFLICT(owner, txid, vout) DO NOTHING`,
		owners,
		t.Outpoint,
	); err != nil {
		log.Panicln("insOwner Err:", err)
		return err
	}

	return err
}

func (t *Txo) SaveSpend(ctx context.Context) (err error) {
	if err = t.EnsureTxn(ctx); err != nil {
		return
	}
	for i := 0; i < 3; i++ {
		log.Printf("SaveSpend %s %s\n", t.Outpoint, t.Spend)
		if _, err = Db.Exec(ctx, `
			INSERT INTO txos(outpoint, txid, vout, satoshis, height, idx, spend, spend_height, spend_idx)
			SELECT $1, t.txid, $3, $4, t.height, t.idx, s.txid, s.height, s.idx
			FROM txns t, txns s
			WHERE t.txid=$2 AND s.txid=$5
			ON CONFLICT(outpoint) DO UPDATE SET
				spend=EXCLUDED.spend,
				spend_height=EXCLUDED.spend_height,
				spend_idx=EXCLUDED.spend_idx`,
			t.Outpoint,
			t.Outpoint.Txid(),
			t.Outpoint.Vout(),
			t.Satoshis,
			t.Spend,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					time.Sleep(100 * time.Millisecond)
					log.Printf("Conflict. Retrying SaveSpend %s\n", t.Outpoint)
					continue
				}
			}
			log.Panicln("saveSpend Err:", err)
			return
		}
		break
	}
	if err != nil {
		log.Panicln("insTxo Err:", err)
	}
	return err
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
