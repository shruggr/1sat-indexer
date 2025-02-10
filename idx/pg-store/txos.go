package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
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

func (p *PGStore) SaveTxo(ctx context.Context, txo *idx.Txo, height uint32, blkIdx uint64) error {
	outpoint := txo.Outpoint.String()
	score := idx.HeightScore(height, blkIdx)

	if _, err := p.DB.Exec(ctx, `INSERT INTO txos(outpoint, height, idx, satoshis, owners)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (outpoint) DO UPDATE SET height = $2, idx = $3, satoshis = $4, owners = $5`,
		outpoint,
		height,
		blkIdx,
		*txo.Satoshis,
		txo.Owners,
	); err != nil {
		log.Panic(err)
		return err
	} else {
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			if _, err := p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
				VALUES ($1, $2, $3)
				ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
				idx.OwnerKey(owner),
				outpoint,
				score,
			); err != nil {
				log.Panic(err)
				return err
			}
		}
	}

	if err := p.SaveTxoData(ctx, txo); err != nil {
		log.Panic(err)
		return err
	}

	for _, owner := range txo.Owners {
		evt.Publish(ctx, idx.OwnerKey(owner), outpoint)
	}
	return nil
}

func (p *PGStore) SaveSpend(ctx context.Context, spend *idx.Txo, txid string, height uint32, blkIdx uint64) error {
	score := idx.HeightScore(height, blkIdx)
	if _, err := p.DB.Exec(ctx, `INSERT INTO txos(outpoint, spend)
		VALUES ($1, $2)
		ON CONFLICT (outpoint) DO UPDATE SET spend = $2`,
		spend.Outpoint.String(),
		txid,
	); err != nil {
		log.Panic(err)
		return err
	} else {
		for _, owner := range spend.Owners {
			if owner == "" {
				continue
			}
			if _, err := p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
				VALUES ($1, $2, $3)
				ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
				idx.OwnerKey(owner),
				txid,
				score,
			); err != nil {
				log.Panic(err)
				return err
			}
		}
	}
	for _, owner := range spend.Owners {
		evt.Publish(ctx, idx.OwnerKey(owner), txid)
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
				WHERE search_key = ANY($1) AND member = $2`,
			spend.Owners,
			txid,
		); err != nil {
			log.Panic(err)
			return err
		}
	}

	return nil
}

func (p *PGStore) SaveTxoData(ctx context.Context, txo *idx.Txo) (err error) {
	if len(txo.Data) == 0 {
		return nil
	}
	outpoint := txo.Outpoint.String()
	score := idx.HeightScore(txo.Height, txo.Idx)
	for tag, data := range txo.Data {
		if _, err := p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
			VALUES ($1, $2, $3)
			ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
			evt.TagKey(tag),
			outpoint,
			score,
		); err != nil {
			log.Panic(err)
			return err
		}
		for _, event := range data.Events {
			if _, err := p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
				VALUES ($1, $2, $3)
				ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
				evt.EventKey(tag, event),
				outpoint,
				score,
			); err != nil {
				log.Panic(err)
				return err
			}
		}
		if idxData, err := data.MarshalJSON(); err != nil {
			log.Panic(err)
			return err
		} else if _, err := p.DB.Exec(ctx, `INSERT INTO txo_data(outpoint, tag, data)
			VALUES ($1, $2, $3)
			ON CONFLICT (outpoint, tag) DO UPDATE SET data = $3`,
			outpoint,
			tag,
			idxData,
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
	if result, err := p.DB.Exec(ctx, `INSERT INTO txos(outpoint, spend)
		VALUES ($1, $2)
		ON CONFLICT (outpoint) DO UPDATE 
			SET spend = $2 
			WHERE txos.spend = ''`,
		outpoint,
		txid,
	); err != nil {
		log.Panic(err)
		return false, err
	} else {
		return result.RowsAffected() > 0, nil
	}
}

func (p *PGStore) UnsetSpends(ctx context.Context, outpoints []string) error {
	if _, err := p.DB.Exec(ctx, `UPDATE txos 
		SET spend = ''
		WHERE outpoint = ANY($1)`,
		outpoints,
	); err != nil {
		log.Panic(err)
		return err
	}
	return nil
}

func (p *PGStore) RollbackTxo(ctx context.Context, txo *idx.Txo) error {
	outpoint := txo.Outpoint.String()
	if accounts, err := p.AcctsByOwners(ctx, txo.Owners); err != nil {
		log.Panic(err)
		return err
	} else if t, err := p.DB.Begin(ctx); err != nil {
		log.Panic(err)
		return err
	} else {
		defer t.Rollback(ctx)
		keys := make([]string, 0, len(txo.Owners)+len(accounts))
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			keys = append(keys, idx.OwnerKey(owner))
		}
		for _, acct := range accounts {
			keys = append(keys, idx.AccountKey(acct))
		}
		for tag, data := range txo.Data {
			keys = append(keys, evt.TagKey(tag))
			for _, event := range data.Events {
				keys = append(keys, evt.EventKey(tag, event))
			}
		}

		if _, err = t.Exec(ctx, `DELETE FROM logs
			WHERE search_key = ANY($1) AND member = $2`,
			keys,
			outpoint,
		); err != nil {
			log.Panic(err)
			return err
		} else if _, err = t.Exec(ctx, `DELETE FROM txo_data
			WHERE outpoint = $1`,
			outpoint,
		); err != nil {
			log.Panic(err)
			return err
		} else if _, err = t.Exec(ctx, `DELETE FROM txos
			WHERE outpoint = $1`,
			outpoint,
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
