package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

func NewSQLiteStore(connString string) (*SQLiteStore, error) {
	if strings.HasPrefix(connString, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("unable to get home directory: %w", err)
		}
		connString = filepath.Join(home, connString[2:])
	}

	if err := os.MkdirAll(filepath.Dir(connString), 0755); err != nil {
		return nil, fmt.Errorf("unable to create database directory: %w", err)
	}

	writeDb, err := sql.Open("sqlite3", connString)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	// Critical: Only one write connection to prevent locking
	writeDb.SetMaxOpenConns(1)

	readDb, err := sql.Open("sqlite3", connString)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	// Allow multiple concurrent readers
	readDb.SetMaxIdleConns(4)
	readDb.SetMaxOpenConns(15)

	// Set PRAGMA commands for better concurrency
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 60000",  // 60 seconds timeout
		"PRAGMA synchronous = NORMAL",  // Better performance, still safe with WAL
		"PRAGMA cache_size = -2000000", // 2GB cache (negative = KB)
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 2147483648", // 2GB memory-mapped I/O
	}

	for _, pragma := range pragmas {
		if _, err := writeDb.Exec(pragma); err != nil {
			log.Panic(err)
			return nil, fmt.Errorf("failed to execute %s on writeDb: %w", pragma, err)
		}
		if _, err := readDb.Exec(pragma); err != nil {
			log.Panic(err)
			return nil, fmt.Errorf("failed to execute %s on readDb: %w", pragma, err)
		}
	}
	if err := initSchema(writeDb); err != nil {
		return nil, fmt.Errorf("unable to initialize schema: %w", err)
	}

	return &SQLiteStore{WRITEDB: writeDb, READDB: readDb}, nil
}

func (s *SQLiteStore) LoadTxo(ctx context.Context, outpoint string, tags []string, script bool, spend bool) (*idx.Txo, error) {
	row := s.READDB.QueryRowContext(ctx, `SELECT outpoint, height, idx, satoshis, owners, spend
		FROM txos WHERE outpoint = ? AND satoshis IS NOT NULL`, outpoint)

	txo := &idx.Txo{}
	var sats sql.NullInt64
	var owners string
	var spendTxid string
	if err := row.Scan(&txo.Outpoint, &txo.Height, &txo.Idx, &sats, &owners, &spendTxid); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	}

	if spend {
		txo.Spend = spendTxid
	}
	if sats.Valid {
		satoshis := uint64(sats.Int64)
		txo.Satoshis = &satoshis
	}
	if err := json.Unmarshal([]byte(owners), &txo.Owners); err != nil {
		log.Panic(err)
		return nil, err
	}
	txo.Score = idx.HeightScore(txo.Height, txo.Idx)

	var err error
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

	return txo, nil
}

func (s *SQLiteStore) LoadTxos(ctx context.Context, outpoints []string, tags []string, script bool, spend bool) ([]*idx.Txo, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}

	query := `SELECT outpoint, height, idx, satoshis, owners, spend
		FROM txos
		WHERE outpoint IN (` + placeholders(len(outpoints)) + `)`

	rows, err := s.READDB.QueryContext(ctx, query, toInterfaceSlice(outpoints)...)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	defer rows.Close()

	txos := make([]*idx.Txo, 0, len(outpoints))
	for rows.Next() {
		txo := &idx.Txo{
			Data: make(idx.IndexDataMap),
		}
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

		if len(tags) > 0 {
			if txo.Data, err = s.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
				log.Panic(err)
				return nil, err
			}
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
	rows, err := s.READDB.QueryContext(ctx, `SELECT outpoint
		FROM txos
		WHERE outpoint LIKE ?`, fmt.Sprintf("%s%%", txid))
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
		outpoints = append(outpoints, outpoint)
	}
	return s.LoadTxos(ctx, outpoints, tags, script, spend)
}

func (s *SQLiteStore) LoadData(ctx context.Context, outpoint string, tags []string) (data idx.IndexDataMap, err error) {
	if len(tags) == 0 {
		return nil, nil
	}
	data = make(idx.IndexDataMap, len(tags))

	query := `SELECT tag, data
		FROM txo_data
		WHERE outpoint=? AND tag IN (` + placeholders(len(tags)) + `)`
	args := append([]interface{}{outpoint}, toInterfaceSlice(tags)...)

	rows, err := s.READDB.QueryContext(ctx, query, args...)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
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
	return data, nil
}

func (s *SQLiteStore) SaveTxos(idxCtx *idx.IndexContext) (err error) {
	ctx := idxCtx.Ctx

	// Prepare batch data
	type txoInsert struct {
		outpoint string
		owners   string
		datas    map[string]any
		events   []string
	}

	txoInserts := make([]txoInsert, 0, len(idxCtx.Txos))
	outpoints := make([]string, 0, len(idxCtx.Txos))

	for _, txo := range idxCtx.Txos {
		outpoint := txo.Outpoint.String()
		outpoints = append(outpoints, outpoint)

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
			return err
		}

		txoInserts = append(txoInserts, txoInsert{
			outpoint: outpoint,
			owners:   string(owners),
			datas:    datas,
			events:   txo.Events,
		})
	}

	// Batch insert txo_data with chunking to avoid SQLite variable limit
	if len(txoInserts) > 0 {
		var dataArgs []interface{}
		var dataPlaceholders []string
		for _, ti := range txoInserts {
			for tag, data := range ti.datas {
				dataPlaceholders = append(dataPlaceholders, "(?, ?, ?)")
				dataArgs = append(dataArgs, ti.outpoint, tag, data)
			}
		}
		if len(dataArgs) > 0 {
			// SQLite has a limit of ~32766 variables per query
			// Each row has 3 variables, so max ~10000 rows per batch
			// Use 5000 as a safe limit to account for overhead
			const maxRowsPerBatch = 5000
			const varsPerRow = 3

			for i := 0; i < len(dataPlaceholders); i += maxRowsPerBatch {
				end := min(i+maxRowsPerBatch, len(dataPlaceholders))

				batchPlaceholders := dataPlaceholders[i:end]
				batchArgs := dataArgs[i*varsPerRow : end*varsPerRow]

				query := "INSERT INTO txo_data(outpoint, tag, data) VALUES " +
					strings.Join(batchPlaceholders, ", ") +
					" ON CONFLICT (outpoint, tag) DO UPDATE SET data = excluded.data"
				if _, err := s.WRITEDB.ExecContext(ctx, query, batchArgs...); err != nil {
					log.Panicln("insert txo_data Err:", err)
					return err
				}
			}
		}
	}

	// Batch insert txos with chunking to avoid SQLite variable limit
	if len(txoInserts) > 0 {
		var txoArgs []interface{}
		var txoPlaceholders []string
		for i, ti := range txoInserts {
			txoPlaceholders = append(txoPlaceholders, "(?, ?, ?, ?, ?)")
			txoArgs = append(txoArgs, ti.outpoint, idxCtx.Height, idxCtx.Idx, *idxCtx.Txos[i].Satoshis, ti.owners)
		}

		// SQLite has a limit of ~32766 variables per query
		// Each row has 5 variables, so use 5000 rows as a safe batch limit
		const maxRowsPerBatch = 5000
		const varsPerRow = 5

		for i := 0; i < len(txoPlaceholders); i += maxRowsPerBatch {
			end := min(i+maxRowsPerBatch, len(txoPlaceholders))

			batchPlaceholders := txoPlaceholders[i:end]
			batchArgs := txoArgs[i*varsPerRow : end*varsPerRow]

			query := "INSERT INTO txos(outpoint, height, idx, satoshis, owners) VALUES " +
				strings.Join(batchPlaceholders, ", ") +
				" ON CONFLICT (outpoint) DO UPDATE SET height = excluded.height, idx = excluded.idx, satoshis = excluded.satoshis, owners = excluded.owners"
			if _, err := s.WRITEDB.ExecContext(ctx, query, batchArgs...); err != nil {
				log.Panicln("insert txos Err:", err)
				return err
			}
		}
	}

	// Batch insert logs with chunking to avoid SQLite variable limit
	if len(txoInserts) > 0 {
		var logArgs []interface{}
		var logPlaceholders []string
		score := idxCtx.Score
		for _, ti := range txoInserts {
			for _, event := range ti.events {
				logPlaceholders = append(logPlaceholders, "(?, ?, ?)")
				logArgs = append(logArgs, event, ti.outpoint, score)
			}
		}
		if len(logArgs) > 0 {
			// SQLite has a limit of ~32766 variables per query
			// Each row has 3 variables, so use 5000 rows as a safe batch limit
			const maxRowsPerBatch = 5000
			const varsPerRow = 3

			for i := 0; i < len(logPlaceholders); i += maxRowsPerBatch {
				end := min(i+maxRowsPerBatch, len(logPlaceholders))

				batchPlaceholders := logPlaceholders[i:end]
				batchArgs := logArgs[i*varsPerRow : end*varsPerRow]

				query := "INSERT INTO logs(search_key, member, score) VALUES " +
					strings.Join(batchPlaceholders, ", ") +
					" ON CONFLICT (search_key, member) DO UPDATE SET score = excluded.score"
				if _, err := s.WRITEDB.ExecContext(ctx, query, batchArgs...); err != nil {
					log.Panicln("insert logs Err:", err)
					return err
				}
			}
		}
	}

	// Publish events
	for vout := range idxCtx.Txos {
		for _, event := range txoInserts[vout].events {
			evt.Publish(ctx, event, outpoints[vout])
		}
	}
	return nil
}

func (s *SQLiteStore) SaveSpends(idxCtx *idx.IndexContext) error {
	ctx := idxCtx.Ctx
	score := idx.HeightScore(idxCtx.Height, idxCtx.Idx)

	owners := make(map[string]struct{}, 10)
	ownerKeys := make([]string, 0, 10)

	// Collect unique owners and prepare spend data
	for _, spend := range idxCtx.Spends {
		for _, owner := range spend.Owners {
			if _, ok := owners[owner]; !ok && owner != "" {
				owners[owner] = struct{}{}
				ownerKey := idx.OwnerKey(owner)
				ownerKeys = append(ownerKeys, ownerKey)
			}
		}
	}

	// Batch update spends in txos table with chunking to avoid SQLite variable limit
	if len(idxCtx.Spends) > 0 {
		var txoArgs []interface{}
		var txoPlaceholders []string
		for _, spend := range idxCtx.Spends {
			outpoint := spend.Outpoint.String()
			txoPlaceholders = append(txoPlaceholders, "(?, ?)")
			txoArgs = append(txoArgs, outpoint, idxCtx.TxidHex)
		}

		const maxRowsPerBatch = 5000
		const varsPerRow = 2

		for i := 0; i < len(txoPlaceholders); i += maxRowsPerBatch {
			end := min(i+maxRowsPerBatch, len(txoPlaceholders))

			batchPlaceholders := txoPlaceholders[i:end]
			batchArgs := txoArgs[i*varsPerRow : end*varsPerRow]

			query := "INSERT INTO txos(outpoint, spend) VALUES " +
				strings.Join(batchPlaceholders, ", ") +
				" ON CONFLICT (outpoint) DO UPDATE SET spend = excluded.spend"
			if _, err := s.WRITEDB.ExecContext(ctx, query, batchArgs...); err != nil {
				log.Panic(err)
				return err
			}
		}
	}

	// Batch insert logs for owner keys with chunking to avoid SQLite variable limit
	if len(ownerKeys) > 0 {
		var logArgs []interface{}
		var logPlaceholders []string
		for _, ownerKey := range ownerKeys {
			logPlaceholders = append(logPlaceholders, "(?, ?, ?)")
			logArgs = append(logArgs, ownerKey, idxCtx.TxidHex, score)
		}

		const maxRowsPerBatch = 5000
		const varsPerRow = 3

		for i := 0; i < len(logPlaceholders); i += maxRowsPerBatch {
			end := min(i+maxRowsPerBatch, len(logPlaceholders))

			batchPlaceholders := logPlaceholders[i:end]
			batchArgs := logArgs[i*varsPerRow : end*varsPerRow]

			query := "INSERT INTO logs(search_key, member, score) VALUES " +
				strings.Join(batchPlaceholders, ", ") +
				" ON CONFLICT (search_key, member) DO UPDATE SET score = excluded.score"
			if _, err := s.WRITEDB.ExecContext(ctx, query, batchArgs...); err != nil {
				log.Panic(err)
				return err
			}
		}
	}

	// Publish events
	for _, ownerKey := range ownerKeys {
		evt.Publish(ctx, ownerKey, idxCtx.TxidHex)
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
	}

	if _, err := s.WRITEDB.ExecContext(ctx, `DELETE FROM logs
		WHERE member = ?`,
		txid,
	); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func (s *SQLiteStore) GetSpend(ctx context.Context, outpoint string, refresh bool) (spend string, err error) {
	err = s.READDB.QueryRowContext(ctx, `SELECT spend FROM txos
		WHERE outpoint = ?`, outpoint).Scan(&spend)

	if err != nil && err != sql.ErrNoRows {
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

	query := `SELECT outpoint, spend FROM txos
		WHERE outpoint IN (` + placeholders(len(outpoints)) + `)`

	rows, err := s.READDB.QueryContext(ctx, query, toInterfaceSlice(outpoints)...)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
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
	return spends, nil
}

func (s *SQLiteStore) SetNewSpend(ctx context.Context, outpoint, txid string) (bool, error) {
	result, err := s.WRITEDB.ExecContext(ctx, `INSERT INTO txos(outpoint, spend)
		VALUES (?, ?)
		ON CONFLICT (outpoint) DO UPDATE
			SET spend = ?
			WHERE txos.spend = ''`,
		outpoint,
		txid,
		txid,
	)
	if err != nil {
		log.Panicln("insert Err:", err)
		return false, err
	}

	changes, err := result.RowsAffected()
	if err != nil {
		log.Panic(err)
		return false, err
	}
	return changes > 0, nil
}

func (s *SQLiteStore) UnsetSpends(ctx context.Context, outpoints []string) error {
	query := `UPDATE txos
		SET spend = ''
		WHERE outpoint IN (` + placeholders(len(outpoints)) + `)`

	if _, err := s.WRITEDB.ExecContext(ctx, query, toInterfaceSlice(outpoints)...); err != nil {
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
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	txidPattern := fmt.Sprintf("%s%%", txid)

	if _, err = tx.ExecContext(ctx, `UPDATE txos
		SET spend = ''
		WHERE spend = ?`,
		txid,
	); err != nil {
		log.Panic(err)
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM logs
		WHERE member LIKE ?`,
		txidPattern,
	); err != nil {
		log.Panic(err)
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM txo_data
		WHERE outpoint LIKE ?`,
		txidPattern,
	); err != nil {
		log.Panic(err)
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM txos
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

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS txos (
		outpoint TEXT PRIMARY KEY,
		height INTEGER DEFAULT (unixepoch()),
		idx BIGINT DEFAULT 0,
		spend TEXT NOT NULL DEFAULT '',
		satoshis BIGINT,
		owners TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_txos_height_idx ON txos (height, idx);
	CREATE INDEX IF NOT EXISTS idx_txos_spend ON txos (spend);

	CREATE TABLE IF NOT EXISTS txo_data (
		outpoint TEXT,
		tag TEXT,
		data TEXT,
		PRIMARY KEY (outpoint, tag)
	);
	CREATE INDEX IF NOT EXISTS idx_txo_data_outpoint_tag ON txo_data (outpoint, tag);

	CREATE TABLE IF NOT EXISTS logs (
		search_key TEXT,
		member TEXT,
		score REAL,
		PRIMARY KEY (search_key, member)
	);
	CREATE INDEX IF NOT EXISTS idx_logs_score ON logs (search_key, score);
	CREATE INDEX IF NOT EXISTS idx_logs_member ON logs (member, score);

	CREATE TABLE IF NOT EXISTS owner_accounts (
		owner TEXT PRIMARY KEY,
		account TEXT,
		sync_height INT DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_owner_accounts_account ON owner_accounts (account);
	`
	_, err := db.Exec(schema)
	return err
}
