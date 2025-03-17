package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type SQLiteStore struct {
	WRITEDB *sql.DB
	READDB  *sql.DB
}

var getTxo *sql.Stmt
var getTxosByTxid *sql.Stmt
var insLog *sql.Stmt
var putLogOnce *sql.Stmt
var getLogScore *sql.Stmt
var setSpend *sql.Stmt
var getSpend *sql.Stmt
var insOwnerAcct *sql.Stmt
var saveTxOuts *sql.Stmt

var pageSize = 1000

func NewSQLiteStore(connString string) (*SQLiteStore, error) {
	writeDb, err := sql.Open("sqlite3", connString)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	writeDb.SetMaxOpenConns(1)

	readDb, err := sql.Open("sqlite3", connString)
	if err != nil {
		log.Panic(err)
		return nil, err
	}

	// Set PRAGMA commands
	if _, err := readDb.Exec("PRAGMA journal_mode = WAL"); err != nil {
		log.Panic(err)
		return nil, err
	} else if _, err := writeDb.Exec("PRAGMA journal_mode = WAL"); err != nil {
		log.Panic(err)
		return nil, err
	} else if _, err := writeDb.Exec("PRAGMA busy_timeout = 10000"); err != nil {
		log.Panic(err)
		return nil, err
	}

	if getTxo, err = readDb.Prepare(`SELECT height, idx, json_extract(outputs, ?)
	    FROM txns
		WHERE txid = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getTxosByTxid, err = readDb.Prepare(`SELECT height, idx, outputs
		FROM txns
		WHERE txid = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if setSpend, err = writeDb.Prepare(`INSERT INTO spends(outpoint, spend, height, idx)
		VALUES (?, ?, ?, ?)
		ON CONFLICT DO UPDATE SET spend=?, height=?, idx=?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getSpend, err = readDb.Prepare(`SELECT spend FROM spends
		WHERE outpoint = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insLog, err = writeDb.Prepare(`INSERT INTO logs(search_key, member, score)
		VALUES (?, ?, ?)
		ON CONFLICT DO UPDATE SET score = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if putLogOnce, err = writeDb.Prepare(`INSERT INTO logs(search_key, member, score)
        VALUES (?, ?, ?)
        ON CONFLICT DO NOTHING`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getLogScore, err = readDb.Prepare(`SELECT score FROM logs
        WHERE search_key = ? AND member = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insOwnerAcct, err = writeDb.Prepare(`INSERT INTO owner_accounts(owner, account)
		VALUES (?, ?)
		ON CONFLICT DO UPDATE SET account = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if saveTxOuts, err = writeDb.Prepare(`INSERT INTO txns(txid, height, idx, outputs)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (txid) DO UPDATE SET height = ?, idx = ?`); err != nil {
		log.Panic(err)
		return nil, err
	}
	return &SQLiteStore{WRITEDB: writeDb, READDB: readDb}, nil
}

func (s *SQLiteStore) LoadTxo(ctx context.Context, outpoint string, tags []string, script bool, spend bool) (*idx.Txo, error) {
	op, err := lib.NewOutpointFromString(outpoint)
	if err != nil {
		return nil, err
	}
	row := getTxo.QueryRowContext(ctx, fmt.Sprintf("$[%d]", op.Vout()), op.TxidHex())
	txo := &idx.Txo{
		Outpoint: op,
		Data:     map[string]*idx.IndexData{},
	}
	tdata := &idx.TxoData{
		Data: map[string]json.RawMessage{},
	}
	if err := row.Scan(&txo.Height, &txo.Idx, &tdata); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else {
		if spend {
			if txo.Spend, err = s.GetSpend(ctx, outpoint, false); err != nil {
				return nil, err
			}
		}

		txo.Satoshis = tdata.Satoshis
		txo.Owners = tdata.Owners
		for _, tag := range tags {
			if data, ok := tdata.Data[tag]; ok && data != nil {
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
	type txResult struct {
		height  uint32
		idx     uint64
		txoData []*idx.TxoData
	}
	txidMap := make(map[string]*txResult, len(outpoints))
	txids := make([]string, 0, len(outpoints))
	txos := make([]*idx.Txo, 0, len(outpoints))
	for _, outpoint := range outpoints {
		txid := outpoint[:64]
		if _, ok := txidMap[txid]; !ok {
			txidMap[txid] = &txResult{}
			txids = append(txids, txid)
		}
	}
	if rows, err := s.READDB.QueryContext(ctx, `SELECT txid, height, idx, outputs
	    FROM txns
		WHERE txid IN (`+placeholders(len(txids))+`)`,
		toInterfaceSlice(txids)...,
	); err != nil {
		log.Panic(err)
		return nil, err
	} else {
		defer rows.Close()
		for rows.Next() {
			var txid string
			var height uint32
			var blkIdx uint64
			var outsStr string
			if err := rows.Scan(&txid, &height, &blkIdx, &outsStr); err == sql.ErrNoRows {
				return nil, nil
			} else if err != nil {
				log.Panic(err)
				return nil, err
			} else if err = json.Unmarshal([]byte(outsStr), &txidMap[txid].txoData); err != nil {
				log.Panic(err)
				return nil, err
			} else {
				txidMap[txid].height = height
				txidMap[txid].idx = blkIdx
			}
		}

		for _, outpoint := range outpoints {
			if op, err := lib.NewOutpointFromString(outpoint); err != nil {
				log.Panic(err)
				return nil, err
			} else {
				txData := txidMap[op.String()]
				txo := &idx.Txo{
					Outpoint: op,
					Height:   txData.height,
					Idx:      txData.idx,
					Data:     map[string]*idx.IndexData{},
				}
				tdata := txData.txoData[op.Vout()]
				txo.Satoshis = tdata.Satoshis
				txo.Owners = tdata.Owners
				for _, tag := range tags {
					if data, ok := tdata.Data[tag]; ok && data != nil {
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

func (s *SQLiteStore) LoadData(ctx context.Context, outpoint string, tags []string) (data idx.IndexDataMap, err error) {
	if txo, err := s.LoadTxo(ctx, outpoint, tags, false, false); err != nil {
		return nil, err
	} else if txo == nil {
		return nil, nil
	} else {
		data = make(idx.IndexDataMap, len(tags))
		for _, tag := range tags {
			if d, ok := txo.Data[tag]; ok {
				data[tag] = d
			}
		}
	}

	return data, nil
}

func (s *SQLiteStore) SaveTxos(idxCtx *idx.IndexContext) (err error) {
	ctx := idxCtx.Ctx
	outpoints := make([]string, 0, len(idxCtx.Txos))
	txOuts := make([]*idx.TxoData, 0, len(idxCtx.Txos))
	eventRows := 0
	for _, txo := range idxCtx.Txos {
		tdata := &idx.TxoData{
			Satoshis: txo.Satoshis,
			Owners:   txo.Owners,
			Data:     map[string]json.RawMessage{},
		}
		outpoints = append(outpoints, txo.Outpoint.String())
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
			evtRows = append(evtRows, "("+placeholders(3)+")")
			evtArgs = append(evtArgs, event, outpoints[vout], idxCtx.Score)
		}
		if len(evtRows) > 0 && (len(evtRows) > pageSize || vout == len(idxCtx.Txos)-1) {
			evtArgs = append(evtArgs, idxCtx.Score)
			sql := "INSERT INTO logs(search_key, member, score) VALUES " +
				strings.Join(evtRows, ", ") +
				" ON CONFLICT DO UPDATE SET score = ?"
			if _, err = s.WRITEDB.ExecContext(ctx, sql,
				evtArgs...,
			); err != nil {
				log.Println(sql)
				log.Panic(err)
				return err
			}
			evtRows = evtRows[:0]
			evtArgs = evtArgs[:0]

		}
	}

	for vout, txo := range idxCtx.Txos {
		for _, event := range txo.Events {
			evt.Publish(ctx, event, outpoints[vout])
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
		spendRows = append(spendRows, "("+placeholders(4)+")")
		spendArgs = append(spendArgs, outpoint, idxCtx.TxidHex, idxCtx.Height, idxCtx.Idx)
		spends[outpoint] = idxCtx.TxidHex
		for _, owner := range spend.Owners {
			if _, ok := owners[owner]; !ok && owner != "" {
				owners[owner] = struct{}{}
				ownerKey := idx.OwnerKey(owner)
				ownerKeys = append(ownerKeys, ownerKey)
				logRows = append(logRows, "("+placeholders(3)+")")
				logArgs = append(logArgs, ownerKey, idxCtx.TxidHex, idxCtx.Score)
			}
		}
	}

	// spendArgs = append(spendArgs, idxCtx.TxidHex, idxCtx.Height, idxCtx.Idx)
	logArgs = append(logArgs, idxCtx.Score)
	if len(spendRows) > 0 {
		for i := 0; i < len(spendRows); i += pageSize {
			sql := "INSERT INTO spends(outpoint, spend, height, idx) VALUES " +
				strings.Join(spendRows[i:min(i+pageSize, len(spendRows))], ", ") +
				" ON CONFLICT DO UPDATE SET spend=?, height=?, idx=?"
			if _, err := s.WRITEDB.ExecContext(idxCtx.Ctx, sql,
				append(spendArgs[i*4:min(i*4+4000, len(spendArgs))], idxCtx.TxidHex, idxCtx.Height, idxCtx.Idx)...,
			); err != nil {
				log.Panic(err)
			}
		}
	}
	if len(logRows) > 0 {
		if _, err := s.WRITEDB.ExecContext(idxCtx.Ctx, `INSERT INTO logs(search_key, member, score)
		VALUES `+strings.Join(logRows, ", ")+`
		ON CONFLICT(search_key, member) DO UPDATE SET score = ?`,
			logArgs...,
		); err != nil {
			log.Panic(err)
		}
	}

	for _, ownerKey := range ownerKeys {
		evt.Publish(idxCtx.Ctx, ownerKey, idxCtx.TxidHex)
	}
	return nil
}

func (s *SQLiteStore) RollbackSpend(ctx context.Context, spend *idx.Txo, txid string) error {
	if _, err := s.WRITEDB.ExecContext(ctx, `UPDATE txos
        SET spend = ''
        WHERE outpoint = ? AND spend = ?`,
		spend.Outpoint.String(),
		txid,
	); err != nil {
		log.Panic(err)
		return err
	} else {
		if _, err := s.WRITEDB.ExecContext(ctx, `DELETE FROM logs
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
	if rows, err := s.READDB.QueryContext(ctx, `SELECT outpoint, spend FROM txos 
        WHERE outpoint IN (`+placeholders(len(outpoints))+`)`,
		toInterfaceSlice(outpoints)...,
	); err != nil {
		log.Panic(err)
		return nil, err
	} else {
		defer rows.Close()
		spendsMap := make(map[string]string, len(outpoints))
		for rows.Next() {
			var outpoint string
			var spend string
			if err = rows.Scan(&outpoint, &spend); err != nil {
				log.Panic(err)
				return nil, err
			}
			spendsMap[outpoint] = spend
		}
		for _, outpoint := range outpoints {
			if spend, ok := spendsMap[outpoint]; ok {
				spends = append(spends, spend)
			} else {
				if refresh {
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
	if _, err := s.WRITEDB.ExecContext(ctx, `DELETE FROM spends 
        WHERE outpoint IN (`+placeholders(len(outpoints))+`)`,
		toInterfaceSlice(outpoints)...,
	); err != nil {
		log.Panic(err)
		return err
	}
	return nil
}

func (s *SQLiteStore) Rollback(ctx context.Context, txid string) error {
	tx, err := s.WRITEDB.BeginTx(ctx, nil)
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
