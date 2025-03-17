package sqlitestore

import (
	"context"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

func (s *SQLiteStore) Search(ctx context.Context, cfg *idx.SearchCfg) (results []*idx.Log, err error) {
	var sqlBuilder strings.Builder
	args := make([]interface{}, 0, 3)
	if cfg.ComparisonType == idx.ComparisonAND && len(cfg.Keys) > 1 {
		sqlBuilder.WriteString(`SELECT logs.member, min(logs.score) as score FROM logs `)
	} else {
		sqlBuilder.WriteString(`SELECT logs.member, logs.score FROM logs `)
	}

	if len(cfg.Keys) == 1 {
		args = append(args, cfg.Keys[0])
		sqlBuilder.WriteString(`WHERE search_key=? `)
	} else {
		args = append(args, toInterfaceSlice(cfg.Keys)...)
		sqlBuilder.WriteString(`WHERE search_key IN (` + placeholders(len(cfg.Keys)) + `) `)
	}
	if cfg.From != nil {
		args = append(args, *cfg.From)
		if cfg.Reverse {
			sqlBuilder.WriteString("AND score < ? ")
		} else {
			sqlBuilder.WriteString("AND score > ? ")
		}
	}

	if cfg.To != nil {
		args = append(args, *cfg.To)
		if cfg.Reverse {
			sqlBuilder.WriteString("AND score > ? ")
		} else {
			sqlBuilder.WriteString("AND score < ? ")
		}
	}

	if cfg.ComparisonType == idx.ComparisonAND && len(cfg.Keys) > 1 {
		args = append(args, len(cfg.Keys))
		sqlBuilder.WriteString("GROUP BY logs.member ")
		sqlBuilder.WriteString("HAVING COUNT(1) = ? ")
	}

	if cfg.Reverse {
		sqlBuilder.WriteString("ORDER BY score DESC ")
	} else {
		sqlBuilder.WriteString("ORDER BY score ASC ")
	}

	if cfg.Limit > 0 {
		args = append(args, cfg.Limit)
		sqlBuilder.WriteString("LIMIT ? ")
	}

	sql := sqlBuilder.String()
	var start time.Time
	if cfg.Verbose {
		log.Println(sql, args)
		start = time.Now()
	}
	rows, err := s.WRITEDB.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if cfg.Verbose {
		log.Println("Query time", time.Since(start))
	}
	results = make([]*idx.Log, 0, cfg.Limit)
	for rows.Next() {
		var result idx.Log
		if err = rows.Scan(&result.Member, &result.Score); err != nil {
			return nil, err
		}
		results = append(results, &result)
	}
	if cfg.Verbose {
		log.Println("Results", len(results))
	}
	return results, nil
}

func (s *SQLiteStore) SearchMembers(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	items, err := s.Search(ctx, cfg)
	if err != nil {
		return nil, err
	}
	members := make([]string, 0, len(items))
	for _, item := range items {
		members = append(members, item.Member)
	}
	return members, nil
}

func (s *SQLiteStore) SearchOutpoints(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	items, err := s.Search(ctx, cfg)
	if err != nil {
		return nil, err
	}
	outpoints := make([]string, 0, len(items))
	for _, item := range items {
		if len(item.Member) < 65 {
			continue
		}
		outpoints = append(outpoints, item.Member)
	}
	return outpoints, nil
}

func (s *SQLiteStore) SearchTxos(ctx context.Context, cfg *idx.SearchCfg) (txos []*idx.Txo, err error) {
	if cfg.IncludeTxo {
		var outpoints []string
		if outpoints, err = s.SearchOutpoints(ctx, cfg); err != nil {
			return nil, err
		}
		if cfg.FilterSpent {
			if spends, err := s.GetSpends(ctx, outpoints, cfg.RefreshSpends); err != nil {
				return nil, err
			} else {
				var filtered []string
				for i, outpoint := range outpoints {
					if spends[i] != "" {
						filtered = append(filtered, outpoint)
					}
				}
				outpoints = filtered
			}
			cfg.IncludeSpend = false
		}
		if txos, err = s.LoadTxos(ctx, outpoints, cfg.IncludeTags, cfg.IncludeScript, cfg.IncludeSpend); err != nil {
			return nil, err
		}
	} else {
		results, err := s.Search(ctx, cfg)
		if err != nil {
			return nil, err
		}
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
			if txo.Data, err = s.LoadData(ctx, txo.Outpoint.String(), cfg.IncludeTags); err != nil {
				return nil, err
			}
			if txo.Data == nil {
				txo.Data = make(idx.IndexDataMap)
			}
			if cfg.IncludeScript {
				if err := txo.LoadScript(ctx); err != nil {
					return nil, err
				}
			}
			txos = append(txos, txo)
		}
	}

	return txos, nil
}

func (s *SQLiteStore) SearchBalance(ctx context.Context, cfg *idx.SearchCfg) (balance uint64, err error) {
	cfg.FilterSpent = true
	outpoints, err := s.SearchOutpoints(ctx, cfg)
	if err != nil {
		return 0, err
	}
	txos, err := s.LoadTxos(ctx, outpoints, nil, false, false)
	if err != nil {
		return 0, err
	}
	for _, txo := range txos {
		if txo.Satoshis != nil {
			balance += *txo.Satoshis
		}
	}
	return balance, nil
}

func (s *SQLiteStore) SearchTxns(ctx context.Context, cfg *idx.SearchCfg) (txns []*lib.TxResult, err error) {
	txMap := make(map[float64]*lib.TxResult)
	scores := make([]float64, 0, 1000)

	activity, err := s.Search(ctx, cfg)
	if err != nil {
		return nil, err
	}
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
			scores = append(scores, item.Score)
		}
		if out != nil {
			result.Outputs[*out] = struct{}{}
		}
	}
	slices.Sort(scores)
	results := make([]*lib.TxResult, 0, len(scores))
	for _, score := range scores {
		results = append(results, txMap[score])
	}
	return results, nil
}

func (s *SQLiteStore) Balance(ctx context.Context, key string) (balance int64, err error) {
	// row := s.DB.QueryRowContext(ctx, `SELECT SUM(satoshis)
	// 	FROM logs
	// 	WHERE key = ?`,
	// 	key,
	// )
	// if err = row.Scan(&balance); err != nil {
	// 	return 0, err
	// }
	return
}

func (s *SQLiteStore) CountMembers(ctx context.Context, key string) (count uint64, err error) {
	row := s.WRITEDB.QueryRowContext(ctx, `SELECT COUNT(1)
        FROM logs
        WHERE key = ?`,
		key,
	)
	if err = row.Scan(&count); err != nil {
		return 0, err
	}
	return
}
