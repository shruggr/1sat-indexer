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
)

type SQLiteStore struct {
	WRITEDB *sql.DB
	READDB  *sql.DB
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

	if getTxo, err = readDb.Prepare(`SELECT outpoint, height, idx, satoshis, spend
        FROM txos WHERE outpoint = ? AND satoshis IS NOT NULL`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insTxo, err = writeDb.Prepare(`INSERT INTO txos(outpoint, height, idx, satoshis, owners)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (outpoint)
		DO UPDATE SET height = ?, idx = ?, satoshis = ?, owners = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insLog, err = writeDb.Prepare(`INSERT INTO logs(search_key, member, score)
		VALUES (?, ?, ?)
		ON CONFLICT (search_key, member) DO UPDATE SET score = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insData, err = writeDb.Prepare(`INSERT INTO txo_data(outpoint, tag, data)
		VALUES (?, ?, ?)
		ON CONFLICT (outpoint, tag) DO UPDATE SET data = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getTxosByTxid, err = readDb.Prepare(`SELECT outpoint
		FROM txos
		WHERE outpoint LIKE ?`); err != nil {
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
	} else if setSpend, err = writeDb.Prepare(`INSERT INTO txos(outpoint, spend)
		VALUES (?, ?)
		ON CONFLICT (outpoint) DO UPDATE SET spend = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if getSpend, err = readDb.Prepare(`SELECT spend FROM txos
		WHERE outpoint = ?`); err != nil {
		log.Panic(err)
		return nil, err
	} else if insOwnerAcct, err = writeDb.Prepare(`INSERT INTO owner_accounts(owner, account)
		VALUES (?, ?)
		ON CONFLICT(owner) DO UPDATE SET account = ?`); err != nil {
		log.Panic(err)
		return nil, err
	}
	return &SQLiteStore{WRITEDB: writeDb, READDB: readDb}, nil
}

func (s *SQLiteStore) LoadTxo(ctx context.Context, outpoint string, tags []string, script bool, spend bool) (*idx.Txo, error) {
	row := getTxo.QueryRowContext(ctx, outpoint)
	// s.DB.QueryRowContext(ctx, `SELECT outpoint, height, idx, satoshis, spend
	//     FROM txos WHERE outpoint = ? AND satoshis IS NOT NULL`,
	// 	outpoint,
	// )
	txo := &idx.Txo{}
	var sats sql.NullInt64
	var spendTxid string
	if err := row.Scan(&txo.Outpoint, &txo.Height, &txo.Idx, &sats, &spendTxid); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else {
		if spend {
			txo.Spend = spendTxid
		}
		if sats.Valid {
			satoshis := uint64(sats.Int64)
			txo.Satoshis = &satoshis
		}
		txo.Score = idx.HeightScore(txo.Height, txo.Idx)
		if txo.Data, err = s.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
			log.Panic(err)
			return nil, err
		} else if txo.Data == nil {
			txo.Data = make(idx.IndexDataMap)
		}
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
	rows, err := s.READDB.QueryContext(ctx, `SELECT outpoint, height, idx, satoshis, owners, spend
        FROM txos 
        WHERE outpoint IN (`+placeholders(len(outpoints))+`)`,
		toInterfaceSlice(outpoints)...,
	)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	defer rows.Close()
	txos := make([]*idx.Txo, 0, len(outpoints))
	for rows.Next() {
		txo := &idx.Txo{}
		var spendTxid string
		var owners string
		if err = rows.Scan(&txo.Outpoint, &txo.Height, &txo.Idx, &txo.Satoshis, &owners, &spendTxid); err != nil {
			log.Panic(err)
			return nil, err
		} else if err = json.Unmarshal([]byte(owners), &txo.Owners); err != nil {
			log.Panic(err)
			return nil, err
		}
		if spend {
			txo.Spend = spendTxid
		}
		txo.Score = idx.HeightScore(txo.Height, txo.Idx)
		if txo.Data, err = s.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
			log.Panic(err)
			return nil, err
		} else if txo.Data == nil {
			txo.Data = make(idx.IndexDataMap)
		}
		if script {
			if err = txo.LoadScript(ctx); err != nil {
				return nil, err
			}
		}
		txos = append(txos, txo)
	}
	return txos, nil
}

func (s *SQLiteStore) LoadTxosByTxid(ctx context.Context, txid string, tags []string, script bool, spend bool) ([]*idx.Txo, error) {
	rows, err := getTxosByTxid.QueryContext(ctx, fmt.Sprintf("%s%%", txid))
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	defer rows.Close()
	outpoints := make([]string, 0)
	for rows.Next() {
		var outpoint string
		if err = rows.Scan(&outpoint); err != nil {
			log.Panic(err)
			return nil, err
		}
	}
	return s.LoadTxos(ctx, outpoints, tags, script, spend)
}

func (s *SQLiteStore) LoadData(ctx context.Context, outpoint string, tags []string) (data idx.IndexDataMap, err error) {
	if len(tags) == 0 {
		return nil, nil
	}
	data = make(idx.IndexDataMap, len(tags))

	args := append([]interface{}{outpoint}, toInterfaceSlice(tags)...)
	if rows, err := s.READDB.QueryContext(ctx, `SELECT tag, data
        FROM txo_data
        WHERE outpoint=? AND tag IN (`+placeholders(len(tags))+`)`,
		args...,
	); err != nil {
		log.Panic(err)
		return nil, err
	} else {
		defer rows.Close()
		for rows.Next() {
			var tag string
			var dataStr string
			if err = rows.Scan(&tag, &dataStr); err != nil {
				log.Panic(err)
				return nil, err
			}
			data[tag] = &idx.IndexData{
				Data: json.RawMessage(dataStr),
			}
		}
	}
	return data, nil
}

func (s *SQLiteStore) SaveTxos(idxCtx *idx.IndexContext) (err error) {
	ctx := idxCtx.Ctx
	t, err := s.WRITEDB.Begin()
	defer t.Rollback()

	insTxo, err := t.PrepareContext(ctx, `INSERT INTO txos(outpoint, height, idx, satoshis, owners)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (outpoint)
		DO UPDATE SET height = ?, idx = ?, satoshis = ?, owners = ?`)
	if err != nil {
		log.Panic(err)
	}
	defer insTxo.Close()

	insLog, err := t.PrepareContext(ctx, `INSERT INTO logs(search_key, member, score)
		VALUES (?, ?, ?)
		ON CONFLICT (search_key, member) DO UPDATE SET score = ?`)
	if err != nil {
		log.Panic(err)
	}
	defer insLog.Close()

	insData, err := t.PrepareContext(ctx, `INSERT INTO txo_data(outpoint, tag, data)
		VALUES (?, ?, ?)
		ON CONFLICT (outpoint, tag) DO UPDATE SET data = ?`)
	if err != nil {
		log.Panic(err)
	}
	defer insData.Close()
	outpoints := make([]string, 0, len(idxCtx.Txos))
	for _, txo := range idxCtx.Txos {
		outpoint := txo.Outpoint.String()
		outpoints = append(outpoints, outpoint)
		score := idxCtx.Score

		txo.Events = make([]string, 0, 100)
		datas := make(map[string]any, len(txo.Data))
		for tag, data := range txo.Data {
			if data != nil {
				txo.Events = append(txo.Events, evt.TagKey(tag))
				for _, event := range data.Events {
					txo.Events = append(txo.Events, evt.EventKey(tag, event))
				}
				if data.Data != nil {
					if datas[tag], err = data.MarshalJSON(); err != nil {
						log.Panic(err)
						return err
					}
				}
			}
		}
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			txo.Events = append(txo.Events, idx.OwnerKey(owner))
		}

		owners, err := json.Marshal(txo.Owners)
		if err != nil {
			log.Panic(err)
		}
		if _, err := insTxo.ExecContext(ctx,
			outpoint,
			idxCtx.Height,
			idxCtx.Idx,
			*txo.Satoshis,
			string(owners),
			idxCtx.Height,
			idxCtx.Idx,
			*txo.Satoshis,
			string(owners),
		); err != nil {
			log.Panicln("insert txos Err:", err)
			return err
		}

		for _, event := range txo.Events {
			if _, err := insLog.ExecContext(ctx,
				event,
				outpoint,
				score,
				score,
			); err != nil {
				log.Panicln("insert logs Err:", err)
				return err
			}
		}

		for tag, data := range datas {
			if _, err := insData.ExecContext(ctx,
				outpoint,
				tag,
				data,
				data,
			); err != nil {
				log.Panicln("insert txo_data Err:", err)
				return err
			}
		}
	}
	if err := t.Commit(); err != nil {
		log.Panic(err)
	}
	for vout, txo := range idxCtx.Txos {
		for _, event := range txo.Events {
			evt.Publish(ctx, event, outpoints[vout])
		}
	}
	return nil
}

func (s *SQLiteStore) SaveSpends(idxCtx *idx.IndexContext) error {
	score := idx.HeightScore(idxCtx.Height, idxCtx.Idx)
	spends := make(map[string]string, len(idxCtx.Spends))
	owners := make(map[string]struct{}, 10)
	ownerKeys := make([]string, 0, 10)
	for _, spend := range idxCtx.Spends {
		outpoint := spend.Outpoint.String()
		spends[outpoint] = idxCtx.TxidHex
		for _, owner := range spend.Owners {
			if _, ok := owners[owner]; !ok && owner != "" {
				owners[owner] = struct{}{}
				ownerKey := idx.OwnerKey(owner)
				ownerKeys = append(ownerKeys, ownerKey)
				if _, err := insLog.ExecContext(idxCtx.Ctx, ownerKey, idxCtx.TxidHex, score, score); err != nil {
					// if _, err := s.DB.ExecContext(idxCtx.Ctx, `INSERT INTO logs(search_key, member, score)
					// 	VALUES (?, ?, ?)
					// 	ON CONFLICT (search_key, member) DO UPDATE SET score = ?`,
					// 	ownerKey,
					// 	idxCtx.TxidHex,
					// 	score,
					// 	score,
					// ); err != nil {
					log.Panic(err)
					return err
				}
			}
		}
		if _, err := setSpend.ExecContext(idxCtx.Ctx, outpoint, idxCtx.TxidHex, idxCtx.TxidHex); err != nil {
			// if _, err := s.DB.ExecContext(idxCtx.Ctx, `INSERT INTO txos(outpoint, spend)
			// 	VALUES (?, ?)
			// 	ON CONFLICT (outpoint) DO UPDATE SET spend = ?`,
			// 	outpoint,
			// 	idxCtx.TxidHex,
			// 	idxCtx.TxidHex,
			// ); err != nil {
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
		// if result, err := s.DB.ExecContext(ctx, `INSERT INTO txos(outpoint, spend)
		//     VALUES (?, ?)
		//     ON CONFLICT (outpoint) DO UPDATE
		//         SET spend = ?
		//         WHERE txos.spend = ''`,
		// 	outpoint,
		// 	txid,
		// 	txid,
		// ); err != nil {
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
	if _, err := s.WRITEDB.ExecContext(ctx, `UPDATE txos 
        SET spend = ''
        WHERE outpoint IN (?)`,
		outpoints,
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
