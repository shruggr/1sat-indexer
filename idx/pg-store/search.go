package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type PGResult struct {
	Outpoint string
	Score    float64
}

func BuildQuery(cfg *idx.SearchCfg) *redis.ZRangeBy {
	query := &redis.ZRangeBy{}
	query.Count = int64(cfg.Limit)

	if cfg.Reverse {
		if cfg.From != 0 {
			query.Max = fmt.Sprintf("(%f", cfg.From)
		} else {
			query.Max = "+inf"
		}
		query.Min = "-inf"
	} else {
		if cfg.From != 0 {
			query.Min = fmt.Sprintf("(%f", cfg.From)
		} else {
			query.Min = "-inf"
		}
		query.Max = "+inf"
	}
	return query
}

func (p *PGStore) search(ctx context.Context, cfg *idx.SearchCfg) (results []PGResult, err error) {
	var sqlBuilder strings.Builder
	args := make([]interface{}, 0, 3)
	sqlBuilder.WriteString(`SELECT logs.member, logs.score 
		FROM logs `)
	if cfg.FilterSpent {
		sqlBuilder.WriteString("JOIN txos ON logs.member = txos.outpoint AND txos.spend='' ")
	}

	sqlBuilder.WriteString(`WHERE search_key = $1 `)
	args = append(args, cfg.Key, cfg.From)
	if cfg.Reverse {
		sqlBuilder.WriteString("AND score < $2 ORDER BY score DESC ")
	} else {
		sqlBuilder.WriteString("AND score > $2 ORDER BY score ASC ")
	}
	if cfg.Limit > 0 {
		sqlBuilder.WriteString("LIMIT $3")
		args = append(args, cfg.Limit)
	}

	if rows, err := p.DB.Query(ctx,
		sqlBuilder.String(),
		args...,
	); err != nil {
		return nil, err
	} else {
		defer rows.Close()
		results = make([]PGResult, 0, cfg.Limit)
		for rows.Next() {
			var result PGResult
			if err = rows.Scan(&result.Outpoint, &result.Score); err != nil {
				return nil, err
			}
			results = append(results, result)
		}
	}
	return results, nil
}

func (p *PGStore) SearchMembers(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	if items, err := p.search(ctx, cfg); err != nil {
		return nil, err
	} else {
		outpoints := make([]string, 0, len(items))
		for _, item := range items {
			if len(item.Outpoint) < 65 {
				continue
			}
			outpoints = append(outpoints, item.Outpoint)
		}
		return outpoints, nil
	}
}

func (p *PGStore) SearchTxos(ctx context.Context, cfg *idx.SearchCfg) (txos []*idx.Txo, err error) {
	if cfg.IncludeTxo {
		var outpoints []string
		if outpoints, err = p.SearchMembers(ctx, cfg); err != nil {
			return nil, err
		}
		if txos, err = p.LoadTxos(ctx, outpoints, nil); err != nil {
			return nil, err
		}
	} else {
		if results, err := p.search(ctx, cfg); err != nil {
			return nil, err
		} else {
			txos = make([]*idx.Txo, 0, len(results))
			for _, result := range results {
				if len(result.Outpoint) < 65 {
					continue
				}
				txo := &idx.Txo{
					Height: uint32(result.Score / 1000000000),
					Idx:    uint64(result.Score) % 1000000000,
					Score:  result.Score,
					Data:   make(map[string]*idx.IndexData),
				}
				if txo.Outpoint, err = lib.NewOutpointFromString(result.Outpoint); err != nil {
					return nil, err
				}
				txos = append(txos, txo)
			}
		}
	}
	if len(cfg.IncludeTags) > 0 {
		for _, txo := range txos {
			if txo.Data, err = p.LoadData(ctx, txo.Outpoint.String(), cfg.IncludeTags); err != nil {
				return nil, err
			} else if txo.Data == nil {
				txo.Data = make(idx.IndexDataMap)
			}
		}
	}
	if cfg.IncludeScript {
		for _, txo := range txos {
			if err := txo.LoadScript(ctx); err != nil {
				return txos, err
			}
		}
	}
	return txos, nil
}

func (p *PGStore) SearchTxns(ctx context.Context, cfg *idx.SearchCfg) (txns []*lib.TxResult, err error) {
	results := make([]*lib.TxResult, 0, 1000)
	txMap := make(map[float64]*lib.TxResult)
	if activity, err := p.search(ctx, cfg); err != nil {
		return nil, err
	} else {
		for _, item := range activity {
			var txid string
			var out *uint32
			if len(item.Outpoint) == 64 {
				txid = item.Outpoint
			} else if outpoint, err := lib.NewOutpointFromString(item.Outpoint); err != nil {
				return nil, err
			} else {
				txid = outpoint.TxidHex()
				vout := outpoint.Vout()
				out = &vout
			}
			var result *lib.TxResult
			var ok bool
			if result, ok = txMap[item.Score]; !ok {
				height := uint32(item.Score / 1000000000)
				result = &lib.TxResult{
					Txid:    txid,
					Height:  height,
					Idx:     uint64(item.Score) % 1000000000,
					Outputs: lib.NewOutputMap(),
					Score:   item.Score,
				}
				txMap[item.Score] = result
				results = append(results, result)
			}
			if out != nil {
				result.Outputs[*out] = struct{}{}
			}
		}
	}
	return results, nil
}

func (p *PGStore) Balance(ctx context.Context, key string) (balance int64, err error) {
	// row := p.DB.QueryRow(ctx, `SELECT SUM(satoshis)
	// 	FROM logs
	// 	WHERE key = $1`,
	// 	key,
	// )
	// if err = row.Scan(&balance); err != nil {
	// 	return 0, err
	// }
	return
}
