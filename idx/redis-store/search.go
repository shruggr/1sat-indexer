package redisstore

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
	"github.com/vmihailenco/msgpack/v5"
)

func BuildQuery(cfg *idx.SearchCfg) *redis.ZRangeBy {
	query := &redis.ZRangeBy{}
	query.Count = int64(cfg.Limit)

	if cfg.Reverse {
		if cfg.From != nil {
			query.Max = fmt.Sprintf("(%f", *cfg.From)
		} else {
			query.Max = "+inf"
		}
		query.Min = "-inf"
	} else {
		if cfg.From != nil {
			query.Min = fmt.Sprintf("(%f", *cfg.From)
		} else {
			query.Min = "-inf"
		}
		query.Max = "+inf"
	}
	if cfg.To != nil {
		if cfg.Reverse {
			query.Min = fmt.Sprintf("(%f", *cfg.To)
		} else {
			query.Max = fmt.Sprintf("(%f", *cfg.To)
		}
	}

	return query
}

func (r *RedisStore) search(ctx context.Context, cfg *idx.SearchCfg) (results []redis.Z, err error) {
	query := BuildQuery(cfg)
	if cfg.Reverse {
		return r.DB.ZRevRangeByScoreWithScores(ctx, cfg.Key, query).Result()
	} else {
		return r.DB.ZRangeByScoreWithScores(ctx, cfg.Key, query).Result()
	}
}

func (r *RedisStore) Search(ctx context.Context, cfg *idx.SearchCfg) (results []*idx.SearchResult, err error) {
	redisResults, err := r.search(ctx, cfg)
	if err != nil {
		return nil, err
	}
	results = make([]*idx.SearchResult, 0, len(redisResults))
	for _, redisResult := range redisResults {
		results = append(results, &idx.SearchResult{
			Member: redisResult.Member.(string),
			Score:  redisResult.Score,
		})
	}
	return
}

func (r *RedisStore) filterSpent(ctx context.Context, outpoints []string, refresh bool) ([]string, error) {
	if len(outpoints) == 0 {
		return outpoints, nil
	}
	unspent := make([]string, 0, len(outpoints))
	// if refresh {
	// 	for _, outpoint := range outpoints {
	// 		log.Println("Checking", outpoint)
	// 		if len(outpoint) < 65 {
	// 			continue
	// 		}

	// 		if spend, err := jb.GetSpend(outpoint); err != nil {
	// 			return nil, err
	// 		} else if spend != "" {
	// 			if _, err := r.SetNewSpend(ctx, outpoint, spend); err != nil {
	// 				return nil, err
	// 			}
	// 			log.Println("Spent from JB", outpoint, spend)
	// 		} else {
	// 			log.Println("Unspent", outpoint, spend)
	// 			unspent = append(unspent, outpoint)
	// 		}
	// 		// } else {
	// 		// 	log.Println("Spent", outpoint, spends[i])
	// 		// }
	// 	}
	// } else {
	if spends, err := r.DB.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		for i, outpoint := range outpoints {
			if len(outpoint) < 65 {
				continue
			}
			if spends[i] == nil {
				if refresh {
					if spend, err := jb.GetSpend(outpoint); err != nil {
						return nil, err
					} else if spend != "" {
						if _, err := r.SetNewSpend(ctx, outpoint, spend); err != nil {
							return nil, err
						}
						log.Println("Spent from JB", outpoint, spend)
						continue
					}
				}
				unspent = append(unspent, outpoint)
			}
		}
	}
	// }

	return unspent, nil
}

func (r *RedisStore) SearchMembers(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	if items, err := r.search(ctx, cfg); err != nil {
		return nil, err
	} else {
		members := make([]string, 0, len(items))
		for _, item := range items {
			members = append(members, item.Member.(string))
		}
		return members, nil
	}
}

func (r *RedisStore) SearchOutpoints(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	if items, err := r.search(ctx, cfg); err != nil {
		return nil, err
	} else {
		outpoints := make([]string, 0, len(items))
		for _, item := range items {
			if len(item.Member.(string)) < 65 {
				continue
			}
			outpoints = append(outpoints, item.Member.(string))
		}
		return outpoints, nil
	}
}

func (r *RedisStore) SearchTxos(ctx context.Context, cfg *idx.SearchCfg) (txos []*idx.Txo, err error) {
	results, err := r.search(ctx, cfg)
	if err != nil {
		return nil, err
	}
	outpoints := make([]string, 0, len(results))
	resultMap := make(map[string]*redis.Z, len(results))
	for _, result := range results {
		outpoint := result.Member.(string)
		if len(outpoint) < 65 {
			continue
		}
		resultMap[outpoint] = &result
		outpoints = append(outpoints, outpoint)
	}
	if cfg.FilterSpent {
		if outpoints, err = r.filterSpent(ctx, outpoints, cfg.RefreshSpends); err != nil {
			return nil, err
		}
	}

	if cfg.IncludeTxo {
		if txos, err = r.LoadTxos(ctx, outpoints, nil); err != nil {
			return nil, err
		}
	} else {
		txos = make([]*idx.Txo, 0, len(results))
		for _, outpoint := range outpoints {
			result := resultMap[outpoint]
			txo := &idx.Txo{
				Height: uint32(result.Score / 1000000000),
				Idx:    uint64(result.Score) % 1000000000,
				Score:  result.Score,
				Data:   make(map[string]*idx.IndexData),
			}
			if txo.Outpoint, err = lib.NewOutpointFromString(outpoint); err != nil {
				return nil, err
			}
			txos = append(txos, txo)
		}
	}
	if len(cfg.IncludeTags) > 0 {
		for _, txo := range txos {
			if txo.Data, err = r.LoadData(ctx, txo.Outpoint.String(), cfg.IncludeTags); err != nil {
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

func (r *RedisStore) SearchBalance(ctx context.Context, cfg *idx.SearchCfg) (balance uint64, err error) {
	if outpoints, err := r.SearchOutpoints(ctx, cfg); err != nil {
		return 0, err
	} else if outpoints, err = r.filterSpent(ctx, outpoints, cfg.RefreshSpends); err != nil {
		return 0, err
	} else if txos, err := r.LoadTxos(ctx, outpoints, nil); err != nil {
		return 0, err
	} else {
		for _, txo := range txos {
			if txo.Satoshis != nil {
				balance += *txo.Satoshis
			}
		}
	}

	return
}

func (r *RedisStore) SearchTxns(ctx context.Context, cfg *idx.SearchCfg, keys []string) (txns []*lib.TxResult, err error) {
	txMap := make(map[float64]*lib.TxResult)
	scores := make([]float64, 0, 1000)

	for _, key := range keys {
		cfg.Key = key
		if activity, err := r.Search(ctx, cfg); err != nil {
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
					scores = append(scores, item.Score)
					// results = append(results, result)
				}
				if out != nil {
					result.Outputs[*out] = struct{}{}
				}
			}
		}
	}
	slices.Sort(scores)
	results := make([]*lib.TxResult, 0, len(scores))
	for _, score := range scores {
		results = append(results, txMap[score])
	}
	return results, nil
}

func (r *RedisStore) Balance(ctx context.Context, key string) (balance int64, err error) {
	var outpoints []string
	balanceKey := idx.BalanceKey(key)
	if balance, err = r.DB.Get(ctx, balanceKey).Int64(); err != nil && err != redis.Nil {
		return 0, err
	} else if err != redis.Nil {
		return balance, nil
	} else if outpoints, err = r.SearchOutpoints(ctx, &idx.SearchCfg{Key: key}); err != nil {
		return 0, err
	}
	var msgpacks []interface{}
	if msgpacks, err = r.DB.HMGet(ctx, TxosKey, outpoints...).Result(); err != nil {
		return
	}
	for _, mp := range msgpacks {
		if mp != nil {
			var txo idx.Txo
			if err = msgpack.Unmarshal([]byte(mp.(string)), &txo); err != nil {
				return
			}
			if txo.Satoshis != nil {
				balance += int64(*txo.Satoshis)
			}
		}
	}
	err = r.DB.Set(ctx, balanceKey, balance, 60*time.Second).Err()
	return
}

func (r *RedisStore) CountMembers(ctx context.Context, key string) (count uint64, err error) {
	return r.DB.ZCard(ctx, key).Uint64()
}
