package pgstore

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

func (p *PGStore) Search(ctx context.Context, cfg *idx.SearchCfg) (results []*idx.SearchResult, err error) {
	var sqlBuilder strings.Builder
	args := make([]interface{}, 0, 3)
	sqlBuilder.WriteString(`SELECT logs.member, logs.score FROM logs `)
	if cfg.FilterSpent {
		sqlBuilder.WriteString("JOIN txos ON logs.member = txos.outpoint AND txos.spend='' ")
	}

	args = append(args, cfg.Key)
	sqlBuilder.WriteString(`WHERE search_key = $1 `)
	if cfg.From != nil {
		args = append(args, *cfg.From)
		param := len(args)
		if cfg.Reverse {
			sqlBuilder.WriteString(fmt.Sprintf("AND score < $%d ", param))
		} else {
			sqlBuilder.WriteString(fmt.Sprintf("AND score > $%d ", param))
		}
	}

	if cfg.To != nil {
		args = append(args, *cfg.To)
		param := len(args)
		if cfg.Reverse {
			sqlBuilder.WriteString(fmt.Sprintf("AND score > $%d ", param))
		} else {
			sqlBuilder.WriteString(fmt.Sprintf("AND score < $%d ", param))
		}
	}

	if cfg.Reverse {
		sqlBuilder.WriteString("ORDER BY score DESC ")
	} else {
		sqlBuilder.WriteString("ORDER BY score ASC ")
	}

	sql := sqlBuilder.String()
	if cfg.Verbose {
		log.Println(sql, args)
	}
	if rows, err := p.DB.Query(ctx, sql, args...); err != nil {
		return nil, err
	} else {
		defer rows.Close()
		results = make([]*idx.SearchResult, 0, cfg.Limit)
		for rows.Next() {
			var result idx.SearchResult
			if err = rows.Scan(&result.Member, &result.Score); err != nil {
				return nil, err
			}
			results = append(results, &result)
		}
	}
	return results, nil
}

func (p *PGStore) SearchMembers(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	if items, err := p.Search(ctx, cfg); err != nil {
		return nil, err
	} else {
		members := make([]string, 0, len(items))
		for _, item := range items {
			members = append(members, item.Member)
		}
		return members, nil
	}
}

func (p *PGStore) SearchOutpoints(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	if items, err := p.Search(ctx, cfg); err != nil {
		return nil, err
	} else {
		outpoints := make([]string, 0, len(items))
		for _, item := range items {
			if len(item.Member) < 65 {
				continue
			}
			outpoints = append(outpoints, item.Member)
		}
		return outpoints, nil
	}
}

func (p *PGStore) SearchTxos(ctx context.Context, cfg *idx.SearchCfg) (txos []*idx.Txo, err error) {
	if cfg.IncludeTxo {
		var outpoints []string
		if outpoints, err = p.SearchOutpoints(ctx, cfg); err != nil {
			return nil, err
		}
		if txos, err = p.LoadTxos(ctx, outpoints, nil); err != nil {
			return nil, err
		}
	} else {
		if results, err := p.Search(ctx, cfg); err != nil {
			return nil, err
		} else {
			txos = make([]*idx.Txo, 0, len(results))
			for _, result := range results {
				txo := &idx.Txo{
					Height: uint32(result.Score / 1000000000),
					Idx:    uint64(result.Score) % 1000000000,
					Score:  result.Score,
					Data:   make(map[string]*idx.IndexData),
				}
				if txo.Outpoint, err = lib.NewOutpointFromString(result.Member); err != nil {
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
	if activity, err := p.Search(ctx, cfg); err != nil {
		return nil, err
	} else {
		for _, item := range activity {
			var txid string
			var out *uint32
			if len(item.Member) == 64 {
				txid = item.Member
			} else if outpoint, err := lib.NewOutpointFromString(item.Member); err != nil {
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
				if cfg.IncludeRawtx {
					if result.Rawtx, err = jb.LoadRawtx(ctx, txid); err != nil {
						return nil, err
					}
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

func (p *PGStore) CountMembers(ctx context.Context, key string) (count uint64, err error) {
	row := p.DB.QueryRow(ctx, `SELECT COUNT(1)
		FROM logs
		WHERE key = $1`,
		key,
	)
	if err = row.Scan(&count); err != nil {
		return 0, err
	}
	return
}
