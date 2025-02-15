package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

type PGStore struct {
	DB *pgxpool.Pool
}

func NewPGStore(connString string) (*PGStore, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	config.MaxConnIdleTime = 15 * time.Second

	db, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Panic(err)
		return nil, err
	}

	return &PGStore{DB: db}, nil
}

func (p *PGStore) insert(ctx context.Context, sql string, args ...interface{}) (resp pgconn.CommandTag, err error) {
	for i := range 3 {
		if resp, err = p.DB.Exec(ctx, sql, args...); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.Code == "23505" {
					log.Println("Conflict. Retrying:", i, err)
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}
			log.Panicln("insTxo Err:", err)
			break
		} else {
			break
		}
	}
	return
}

func (p *PGStore) LoadTxo(ctx context.Context, outpoint string, tags []string, script bool) (*idx.Txo, error) {
	row := p.DB.QueryRow(ctx, `SELECT outpoint, height, idx, satoshis, owners
		FROM txos WHERE outpoint = $1 AND satoshis IS NOT NULL`,
		outpoint,
	)
	txo := &idx.Txo{}
	var sats sql.NullInt64
	if err := row.Scan(&txo.Outpoint, &txo.Height, &txo.Idx, &sats, &txo.Owners); err == pgx.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else {
		if sats.Valid {
			satoshis := uint64(sats.Int64)
			txo.Satoshis = &satoshis
		}
		txo.Score = idx.HeightScore(txo.Height, txo.Idx)
		if txo.Data, err = p.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
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

func (p *PGStore) LoadTxos(ctx context.Context, outpoints []string, tags []string, script bool) ([]*idx.Txo, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}
	rows, err := p.DB.Query(ctx, `SELECT outpoint, height, idx, satoshis, owners
		FROM txos 
		WHERE outpoint = ANY($1)`,
		outpoints,
	)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	defer rows.Close()
	txos := make([]*idx.Txo, 0, len(outpoints))
	for rows.Next() {
		txo := &idx.Txo{}
		if err = rows.Scan(&txo.Outpoint, &txo.Height, &txo.Idx, &txo.Satoshis, &txo.Owners); err != nil {
			log.Panic(err)
			return nil, err
		}
		txo.Score = idx.HeightScore(txo.Height, txo.Idx)
		if txo.Data, err = p.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
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

func (p *PGStore) LoadTxosByTxid(ctx context.Context, txid string, tags []string, script bool) ([]*idx.Txo, error) {
	rows, err := p.DB.Query(ctx, `SELECT outpoint
		FROM txos 
		WHERE outpoint LIKE $1`,
		fmt.Sprintf("%s%%", txid),
	)
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
	return p.LoadTxos(ctx, outpoints, tags, script)
}

func (p *PGStore) LoadData(ctx context.Context, outpoint string, tags []string) (data idx.IndexDataMap, err error) {
	if len(tags) == 0 {
		return nil, nil
	}
	data = make(idx.IndexDataMap, len(tags))
	if rows, err := p.DB.Query(ctx, `SELECT tag, data
		FROM txo_data
		WHERE outpoint=$1 AND tag=ANY($2)`,
		outpoint,
		tags,
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

func (p *PGStore) SaveTxos(idxCtx *idx.IndexContext) error {
	for _, txo := range idxCtx.Txos {
		if err := p.saveTxo(idxCtx.Ctx, txo, idxCtx.Height, idxCtx.Idx); err != nil {
			log.Panic(err)
			return err
		}
	}
	return nil
}

func (p *PGStore) saveTxo(ctx context.Context, txo *idx.Txo, height uint32, blkIdx uint64) (err error) {
	outpoint := txo.Outpoint.String()
	score := idx.HeightScore(height, blkIdx)

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

	if _, err := p.insert(ctx, `INSERT INTO txos(outpoint, height, idx, satoshis, owners)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (outpoint)
			DO UPDATE SET height = $2, idx = $3, satoshis = $4, owners = $5`,
		outpoint,
		height,
		blkIdx,
		*txo.Satoshis,
		txo.Owners,
	); err != nil {
		log.Panicln("insTxo Err:", err)
		return err
	}

	if len(txo.Events) > 0 {
		if _, err := p.insert(ctx, `INSERT INTO logs(search_key, member, score)
			SELECT search_key, $2, $3
			FROM unnest($1::text[]) search_key
			ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
			txo.Events,
			outpoint,
			score,
		); err != nil {
			log.Panicln("insTxo Err:", err)
			return err
		}
	}
	for tag, data := range datas {
		if _, err := p.insert(ctx, `INSERT INTO txo_data(outpoint, tag, data)
				VALUES ($1, $2, $3)
				ON CONFLICT (outpoint, tag) DO UPDATE SET data = $3`,
			outpoint,
			tag,
			data,
		); err != nil {
			log.Panic(err)
			return err
		}
	}

	for _, event := range txo.Events {
		evt.Publish(ctx, event, outpoint)
	}
	return nil
}

func (p *PGStore) SaveSpends(idxCtx *idx.IndexContext) error {
	score := idx.HeightScore(idxCtx.Height, idxCtx.Idx)
	spends := make(map[string]string, len(idxCtx.Spends))
	outpoints := make([]interface{}, 0, len(idxCtx.Spends))
	owners := make(map[string]struct{}, 10)
	ownerKyes := make([]string, 0, 10)
	for _, spend := range idxCtx.Spends {
		outpoint := spend.Outpoint.String()
		spends[outpoint] = idxCtx.TxidHex
		outpoints = append(outpoints, outpoint)
		for _, owner := range spend.Owners {
			if _, ok := owners[owner]; !ok && owner != "" {
				owners[owner] = struct{}{}
				ownerKyes = append(ownerKyes, idx.OwnerKey(owner))
			}
		}
	}
	if _, err := p.insert(idxCtx.Ctx, `INSERT INTO txos(outpoint, spend)
		SELECT outpoint, $2
		FROM unnest($1::text[]) outpoint
		ON CONFLICT (outpoint) DO UPDATE SET spend = $2`,
		outpoints,
		idxCtx.TxidHex,
	); err != nil {
		log.Panic(err)
		return err
	} else if _, err := p.insert(idxCtx.Ctx, `INSERT INTO logs(search_key, member, score)
		SELECT search_key, $2, $3
		FROM unnest($1::text[]) search_key
		ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
		ownerKyes,
		idxCtx.TxidHex,
		score,
	); err != nil {
		log.Panic(err)
		return err
	}

	for _, ownerKey := range ownerKyes {
		evt.Publish(idxCtx.Ctx, ownerKey, idxCtx.TxidHex)
	}
	return nil
}

func (p *PGStore) RollbackSpend(ctx context.Context, spend *idx.Txo, txid string) error {
	if _, err := p.DB.Exec(ctx, `UPDATE txos
		SET spend = ''
		WHERE outpoint = $1 AND spend = $2`,
		spend.Outpoint.String(),
		txid,
	); err != nil {
		log.Panic(err)
		return err
	} else {
		if _, err := p.DB.Exec(ctx, `DELETE FROM logs
			WHERE member = $1`,
			txid,
		); err != nil {
			log.Panic(err)
			return err
		}
	}

	return nil
}

func (p *PGStore) GetSpend(ctx context.Context, outpoint string, refresh bool) (spend string, err error) {
	if err := p.DB.QueryRow(ctx, `SELECT spend FROM txos 
		WHERE outpoint = $1`,
		outpoint,
	).Scan(&spend); err != nil && err != pgx.ErrNoRows {
		log.Panic(err)
		return spend, err
	}
	if spend == "" && refresh {
		if spend, err = jb.GetSpend(outpoint); err != nil {
			return
		} else if spend != "" {
			if _, err = p.SetNewSpend(ctx, outpoint, spend); err != nil {
				return
			}
		}
	}
	return spend, nil
}

func (p *PGStore) GetSpends(ctx context.Context, outpoints []string, refresh bool) ([]string, error) {
	spends := make([]string, 0, len(outpoints))
	if rows, err := p.DB.Query(ctx, `SELECT outpoint, spend FROM txos 
		WHERE outpoint = ANY($1)`,
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
					if _, err = p.SetNewSpend(ctx, outpoint, spend); err != nil {
						return nil, err
					}
				}
			}
			spends = append(spends, spend)
		}
	}
	return spends, nil
}

func (p *PGStore) SetNewSpend(ctx context.Context, outpoint, txid string) (bool, error) {
	if result, err := p.insert(ctx, `INSERT INTO txos(outpoint, spend)
		VALUES ($1, $2)
		ON CONFLICT (outpoint) DO UPDATE 
			SET spend = $2 
			WHERE txos.spend = ''`,
		outpoint,
		txid,
	); err != nil {
		log.Panicln("insTxo Err:", err)
		return false, err
	} else {
		return result.RowsAffected() > 0, nil
	}
}

func (p *PGStore) UnsetSpends(ctx context.Context, outpoints []string) error {
	if _, err := p.insert(ctx, `UPDATE txos 
		SET spend = ''
		WHERE outpoint = ANY($1)`,
		outpoints,
	); err != nil {
		log.Panic(err)
		return err
	}
	return nil
}

func (p *PGStore) Rollback(ctx context.Context, txid string) error {
	if t, err := p.DB.Begin(ctx); err != nil {
		log.Panic(err)
		return err
	} else {
		defer t.Rollback(ctx)
		txidPattern := fmt.Sprintf("%s%%", txid)
		if _, err = t.Exec(ctx, `UPDATE txos
			SET spend = ''
			WHERE spend = $1`,
			txid,
		); err != nil {
			log.Panic(err)
			return err
		} else if _, err = t.Exec(ctx, `DELETE FROM logs
			WHERE member LIKE $1`,
			txidPattern,
		); err != nil {
			log.Panic(err)
			return err
		} else if _, err = t.Exec(ctx, `DELETE FROM txo_data
			WHERE outpoint LIKE $1`,
			txidPattern,
		); err != nil {
			log.Panic(err)
			return err
		} else if _, err = t.Exec(ctx, `DELETE FROM txos
			WHERE outpoint LIKE $1`,
			txidPattern,
		); err != nil {
			log.Panic(err)
			return err
		}

		if err = t.Commit(ctx); err != nil {
			log.Panic(err)
			return err
		}
	}
	return nil
}

// func (p *PGStore) RollbackTxo(ctx context.Context, txo *idx.Txo) error {
// 	outpoint := txo.Outpoint.String()
// 	if accounts, err := p.AcctsByOwners(ctx, txo.Owners); err != nil {
// 		log.Panic(err)
// 		return err
// 	} else if t, err := p.DB.Begin(ctx); err != nil {
// 		log.Panic(err)
// 		return err
// 	} else {
// 		defer t.Rollback(ctx)
// 		keys := make([]string, 0, len(txo.Owners)+len(accounts))
// 		for _, owner := range txo.Owners {
// 			if owner == "" {
// 				continue
// 			}
// 			keys = append(keys, idx.OwnerKey(owner))
// 		}
// 		for _, acct := range accounts {
// 			keys = append(keys, idx.AccountKey(acct))
// 		}
// 		for tag, data := range txo.Data {
// 			keys = append(keys, evt.TagKey(tag))
// 			for _, event := range data.Events {
// 				keys = append(keys, evt.EventKey(tag, event))
// 			}
// 		}

// 		if _, err = t.Exec(ctx, `DELETE FROM logs
// 			WHERE search_key = ANY($1) AND member = $2`,
// 			keys,
// 			outpoint,
// 		); err != nil {
// 			log.Panic(err)
// 			return err
// 		} else if _, err = t.Exec(ctx, `DELETE FROM txo_data
// 			WHERE outpoint = $1`,
// 			outpoint,
// 		); err != nil {
// 			log.Panic(err)
// 			return err
// 		} else if _, err = t.Exec(ctx, `DELETE FROM txos
// 			WHERE outpoint = $1`,
// 			outpoint,
// 		); err != nil {
// 			log.Panic(err)
// 			return err
// 		}

// 		if err = t.Commit(ctx); err != nil {
// 			log.Panic(err)
// 			return err
// 		}
// 	}
// 	return nil
// }
