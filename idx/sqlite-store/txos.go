package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type SQLiteStore struct {
	DB *sql.DB
}

var getTxo *sql.Stmt
var getTxosByTxid *sql.Stmt
var insTxo *sql.Stmt
var insLog *sql.Stmt
var insData *sql.Stmt
var putLogOnce *sql.Stmt
var getLogScore *sql.Stmt
var setSpend *sql.Stmt
var getSpend *sql.Stmt
var insOwnerAcct *sql.Stmt
var saveTxOuts *sql.Stmt

var pageSize = 1000

func NewSQLiteStore(connString string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", connString)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(15 * time.Second)

	// Set PRAGMA commands
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		log.Panic(err)
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		log.Panic(err)
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 10000"); err != nil {
		log.Panic(err)
		return nil, err
	}

	if getTxo, err = db.Prepare(`SELECT txid, height, idx, json_extract(outputs, ?)
	    FROM tx_outs
		WHERE txid = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getTxosByTxid, err = db.Prepare(`SELECT height, idx, outputs
		FROM tx_outs
		WHERE txid = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if setSpend, err = db.Prepare(`INSERT INTO spends(outpoint, spend, height, idx)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (outpoint) DO UPDATE SET spend=?, height=?, idx=?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getSpend, err = db.Prepare(`SELECT spend FROM spends
		WHERE outpoint = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insLog, err = db.Prepare(`INSERT INTO logs(search_key, member, score)
		VALUES (?, ?, ?)
		ON CONFLICT (search_key, member) DO UPDATE SET score = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if putLogOnce, err = db.Prepare(`INSERT INTO logs(search_key, member, score)
        VALUES (?, ?, ?)
        ON CONFLICT DO NOTHING`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getLogScore, err = db.Prepare(`SELECT score FROM logs
        WHERE search_key = ? AND member = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insOwnerAcct, err = db.Prepare(`INSERT INTO owner_accounts(owner, account)
		VALUES (?, ?)
		ON CONFLICT(owner) DO UPDATE SET account = ?`); err != nil {
		log.Panic(err)
		return nil, err
		// } else if saveTxOuts, err = db.Prepare(`INSERT INTO tx_outs(txid, height, idx, outputs)
		// 	VALUES (?, ?, ?, ?)
		// 	ON CONFLICT (txid) DO UPDATE SET height = ?, idx = ?`); err != nil {
		// 	log.Panic(err)
		// 	return nil, err
	}
	return &SQLiteStore{DB: db}, nil
}

func (s *SQLiteStore) LoadTxo(ctx context.Context, outpoint string, tags []string, script bool, spend bool) (*idx.Txo, error) {
	row := getTxo.QueryRowContext(ctx, outpoint)
	txo := &idx.Txo{}
	var txid string
	var outStr string
	tdada := &idx.TxoData{}
	if err := row.Scan(&txid, &txo.Height, &txo.Idx, &outStr); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else if err = json.Unmarshal([]byte(outStr), &tdada); err != nil {
		if spend {
			if txo.Spend, err = s.GetSpend(ctx, outpoint, false); err != nil {
				return nil, err
			}
		}

		txo.Satoshis = tdada.Satoshis
		txo.Owners = tdada.Owners
		for _, tag := range tags {
			if data, ok := tdada.Data[tag]; ok {
				txo.Data[tag] = &idx.IndexData{
					Data: data,
				}
			}
		}
		txo.Score = idx.HeightScore(txo.Height, txo.Idx)
		if script {
			if err = txo.LoadScript(ctx); err != nil {
				return nil, err
			}
		}
	}
	return txo, nil
}

func (s *SQLiteStore) LoadTxos(ctx context.Context, outpoints []string, tags []string, script bool, spend bool) ([]*idx.Txo, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}
	txos := make([]*idx.Txo, 0, len(outpoints))
	for _, outpoint := range outpoints {
		if txo, err := s.LoadTxo(ctx, outpoint, tags, script, spend); err != nil {
			return nil, err
		} else if txo != nil {
			txos = append(txos, txo)
		}
	}

	return txos, nil
}

func (s *SQLiteStore) LoadTxosByTxid(ctx context.Context, txid string, tags []string, script bool, spend bool) ([]*idx.Txo, error) {
	row := getTxosByTxid.QueryRowContext(ctx, txid)
	outpoints := make([]string, 0)
	var height uint32
	var blkIdx uint64
	var outsStr string
	outs := []idx.TxoData{}
	txos := make([]*idx.Txo, 0, 10)
	if err := row.Scan(&txid, &height, &blkIdx, &outsStr); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else if err = json.Unmarshal([]byte(outsStr), &outs); err != nil {
		score := idx.HeightScore(height, blkIdx)
		for vout, tdada := range outs {
			op := fmt.Sprintf("%s_%d", txid, vout)
			outpoints = append(outpoints, op)
			txo := &idx.Txo{
				Height: height,
				Idx:    blkIdx,
				Score:  score,
			}
			if txo.Outpoint, err = lib.NewOutpointFromString(op); err != nil {
				log.Panic(err)
				return nil, err
			}
			txo.Satoshis = tdada.Satoshis
			txo.Owners = tdada.Owners
			for _, tag := range tags {
				if data, ok := tdada.Data[tag]; ok {
					txo.Data[tag] = &idx.IndexData{
						Data: data,
					}
				}
			}

			txos = append(txos, txo)
		}
	}

	if spend {
		if spends, err := s.GetSpends(ctx, outpoints, false); err != nil {
			return nil, err
		} else {
			for i, txo := range txos {
				txo.Spend = spends[i]
			}
		}
	}
	if script {
		if tx, err := jb.LoadTx(ctx, txid, false); err != nil {
			return nil, err
		} else {
			for _, txo := range txos {
				txo.Script = *tx.Outputs[txo.Outpoint.Vout()].LockingScript
			}
		}
	}
	return txos, nil
}

func (s *SQLiteStore) SaveTxos(idxCtx *idx.IndexContext) (err error) {
	ctx := idxCtx.Ctx
	t, err := s.DB.Begin()
	defer t.Rollback()
	outpoints := make([]string, 0, len(idxCtx.Txos))
	txOuts := make([]*idx.TxoData, len(idxCtx.Txos))
	eventRows := 0
	for _, txo := range idxCtx.Txos {
		tdata := &idx.TxoData{
			Satoshis: txo.Satoshis,
			Owners:   txo.Owners,
			Data:     map[string]json.RawMessage{},
		}
		txOuts = append(txOuts, tdata)
		for tag, data := range txo.Data {
			txo.Events = append(txo.Events, evt.TagKey(tag))
			for _, event := range data.Events {
				txo.Events = append(txo.Events, evt.EventKey(tag, event))
			}
			if tdata.Data[tag], err = data.MarshalJSON(); err != nil {
				log.Panic(err)
				return err
			}
		}
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			txo.Events = append(txo.Events, idx.OwnerKey(owner))
		}

		eventRows += len(txo.Events)
	}

	if outs, err := json.Marshal(txOuts); err != nil {
		log.Panic(err)
	} else {
		if _, err = saveTxOuts.ExecContext(ctx, idxCtx.TxidHex, idxCtx.Height, idxCtx.Idx, outs, idxCtx.Height, idxCtx.Idx); err != nil {
			log.Panic(err)
			return err
		}
	}
	evtRows := make([]string, 0, len(idxCtx.Txos))
	evtArgs := make([]interface{}, 0, len(idxCtx.Txos)*3)
	for vout, txo := range idxCtx.Txos {
		for _, event := range txo.Events {
			evt.Publish(ctx, event, outpoints[vout])
			evtRows = append(evtRows, "("+placeholders(3)+")")
			evtArgs = append(evtArgs, event, outpoints[vout], idxCtx.Score)
		}
		if len(evtRows) > 1000 || vout == len(idxCtx.Txos)-1 {
			evtArgs = append(evtArgs, idxCtx.Score)
			if _, err = s.DB.ExecContext(ctx, `INSERT INTO logs(search_key, member, score) VALUES `+
				strings.Join(evtRows, ", ")+
				` ON CONFLICT DO UPDATE SET score = ?`,
				evtArgs...,
			); err != nil {
				log.Panic(err)
				return err
			}
			evtRows = evtRows[:0]
			evtArgs = evtArgs[:0]

		}
	}
	return nil
}

func (s *SQLiteStore) SaveSpends(idxCtx *idx.IndexContext) error {
	// score := idx.HeightScore(idxCtx.Height, idxCtx.Idx)
	spends := make(map[string]string, len(idxCtx.Spends))
	owners := make(map[string]struct{}, 10)
	ownerKeys := make([]string, 0, 10)
	spendRows := make([]string, 0, len(idxCtx.Spends))
	spendArgs := make([]interface{}, 0, len(idxCtx.Spends)*4)
	logRows := make([]string, 0, len(idxCtx.Spends))
	logArgs := make([]interface{}, 0, len(idxCtx.Spends)*4+3)
	for _, spend := range idxCtx.Spends {
		outpoint := spend.Outpoint.String()
		spendRows = append(spendRows, "(?, ?, ?, ?)")
		spendArgs = append(spendArgs, outpoint, idxCtx.TxidHex, idxCtx.Height, idxCtx.Idx)
		spends[outpoint] = idxCtx.TxidHex
		for _, owner := range spend.Owners {
			if _, ok := owners[owner]; !ok && owner != "" {
				owners[owner] = struct{}{}
				ownerKey := idx.OwnerKey(owner)
				ownerKeys = append(ownerKeys, ownerKey)
				logRows = append(logRows, "(?, ?, ?)")
				logArgs = append(logArgs, ownerKey, idxCtx.TxidHex, idxCtx.Score)
			}
		}
	}

	if _, err := s.DB.ExecContext(idxCtx.Ctx, `INSERT INTO spends(outpoint, spend, height, idx)
		VALUES `+strings.Join(spendRows, ", ")+`
		ON CONFLICT (outpoint) REPLACE`,
		spendArgs...,
	); err != nil {
		log.Panic(err)
	} else if _, err := s.DB.ExecContext(idxCtx.Ctx, `INSERT INTO logs(search_key, member, score)
		VALUES `+strings.Join(logRows, ", ")+`
		ON CONFLICT(search_key, member) DO NOTHING`,
		logArgs...,
	); err != nil {
		log.Panic(err)
	}

	for _, ownerKey := range ownerKeys {
		evt.Publish(idxCtx.Ctx, ownerKey, idxCtx.TxidHex)
	}
	return nil
}

func (s *SQLiteStore) RollbackSpend(ctx context.Context, spend *idx.Txo, txid string) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE txos
        SET spend = ''
        WHERE outpoint = ? AND spend = ?`,
		spend.Outpoint.String(),
		txid,
	); err != nil {
		log.Panic(err)
		return err
	} else {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM logs
            WHERE member = ?`,
			txid,
		); err != nil {
			log.Panic(err)
			return err
		}
	}

	return nil
}

func (s *SQLiteStore) GetSpend(ctx context.Context, outpoint string, refresh bool) (spend string, err error) {
	if err := getSpend.QueryRowContext(ctx, outpoint).Scan(&spend); err != nil && err != sql.ErrNoRows {
		// if err := s.DB.QueryRowContext(ctx, `SELECT spend FROM txos
		//     WHERE outpoint = ?`,
		// 	outpoint,
		// ).Scan(&spend); err != nil && err != sql.ErrNoRows {
		log.Panic(err)
		return spend, err
	}
	if spend == "" && refresh {
		if spend, err = jb.GetSpend(outpoint); err != nil {
			return
		} else if spend != "" {
			if _, err = s.SetNewSpend(ctx, outpoint, spend); err != nil {
				return
			}
		}
	}
	return spend, nil
}

func (s *SQLiteStore) GetSpends(ctx context.Context, outpoints []string, refresh bool) ([]string, error) {
	spends := make([]string, 0, len(outpoints))
	if rows, err := s.DB.QueryContext(ctx, `SELECT outpoint, spend FROM txos 
        WHERE outpoint IN (?)`,
		outpoints,
	); err != nil {
		log.Panic(err)
		return nil, err
	} else {
		defer rows.Close()
		for rows.Next() {
			var outpoint string
			var spend string
			if err = rows.Scan(&outpoint, &spend); err != nil {
				log.Panic(err)
				return nil, err
			}
			if spend == "" && refresh {
				if spend, err = jb.GetSpend(outpoint); err != nil {
					return nil, err
				} else if spend != "" {
					if _, err = s.SetNewSpend(ctx, outpoint, spend); err != nil {
						return nil, err
					}
				}
			}
			spends = append(spends, spend)
		}
	}
	return spends, nil
}

func (s *SQLiteStore) SetNewSpend(ctx context.Context, outpoint, txid string) (bool, error) {
	if result, err := setSpend.ExecContext(ctx, outpoint, txid, txid); err != nil {
		log.Panicln("insert Err:", err)
		return false, err
	} else if changes, err := result.RowsAffected(); err != nil {
		log.Panic(err)
		return false, err
	} else {
		return changes > 0, nil
	}
}

func (s *SQLiteStore) UnsetSpends(ctx context.Context, outpoints []string) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM spends 
        WHERE outpoint IN (`+placeholders(len(outpoints))+`)`,
		toInterfaceSlice(outpoints)...,
	); err != nil {
		log.Panic(err)
		return err
	}
	return nil
}

func (s *SQLiteStore) Rollback(ctx context.Context, txid string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		log.Panic(err)
		return err
	}
	defer tx.Rollback()

	txidPattern := fmt.Sprintf("%s%%", txid)
	if _, err = tx.ExecContext(ctx, `UPDATE txos
        SET spend = ''
        WHERE spend = ?`,
		txid,
	); err != nil {
		log.Panic(err)
		return err
	} else if _, err = tx.ExecContext(ctx, `DELETE FROM logs
        WHERE member LIKE ?`,
		txidPattern,
	); err != nil {
		log.Panic(err)
		return err
	} else if _, err = tx.ExecContext(ctx, `DELETE FROM txo_data
        WHERE outpoint LIKE ?`,
		txidPattern,
	); err != nil {
		log.Panic(err)
		return err
	} else if _, err = tx.ExecContext(ctx, `DELETE FROM txos
        WHERE outpoint LIKE ?`,
		txidPattern,
	); err != nil {
		log.Panic(err)
		return err
	}

	if err = tx.Commit(); err != nil {
		log.Panic(err)
		return err
	}
	return nil
}

// Helper functions
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return "?" + strings.Repeat(",?", n-1)
}

func toInterfaceSlice(strs []string) []interface{} {
	ifaces := make([]interface{}, len(strs))
	for i, s := range strs {
		ifaces[i] = s
	}
	return ifaces
}
