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

type record struct {
	count int
	score float64
}

func (r *RedisStore) Search(ctx context.Context, cfg *idx.SearchCfg) (records []*idx.Log, err error) {
	query := BuildQuery(cfg)
	// outpointCounts := make(map[string]int)
	outpointSet := make(map[string]*record)
	keyCount := len(cfg.Keys)
	records = make([]*idx.Log, 0, keyCount*int(cfg.Limit))
	for _, key := range cfg.Keys {
		var results []redis.Z
		if cfg.Reverse {
			if results, err = r.DB.ZRevRangeByScoreWithScores(ctx, key, query).Result(); err != nil {
				return nil, err
			}
		} else {
			if results, err = r.DB.ZRangeByScoreWithScores(ctx, key, query).Result(); err != nil {
				return nil, err
			}
		}
		for _, result := range results {
			outpoint := result.Member.(string)
			if len(outpoint) < 65 && (cfg.OutpointsOnly || cfg.FilterSpent) {
				continue
			}
			if keyCount > 1 {
				if _, exists := outpointSet[outpoint]; !exists {
					outpointSet[outpoint] = &record{
						score: result.Score,
						count: 0,
					}
				}
				outpointSet[outpoint].count++
			} else {
				records = append(records, &idx.Log{
					Member: outpoint,
					Score:  result.Score,
				})
			}
		}
	}
	if keyCount > 1 {
		for outpoint, record := range outpointSet {
			if cfg.ComparisonType == idx.ComparisonAND && record.count != keyCount {
				continue
			}
			records = append(records, &idx.Log{
				Member: outpoint,
				Score:  record.score,
			})
		}
		slices.SortFunc(records, func(a, b *idx.Log) int {
			if cfg.Reverse {
				if a.Score > b.Score {
					return -1
				} else if a.Score < b.Score {
					return 1
				}
			} else {
				if a.Score < b.Score {
					return -1
				} else if a.Score > b.Score {
					return 1
				}
			}
			return 0
		})
	}

	if cfg.Limit > 0 && len(records) > int(cfg.Limit) {
		records = records[:cfg.Limit]
	}
	return records, nil
}

func (r *RedisStore) filterSpent(ctx context.Context, outpoints []string, refresh bool) ([]string, error) {
	if len(outpoints) == 0 {
		return outpoints, nil
	}
	unspent := make([]string, 0, len(outpoints))
	if spends, err := r.DB.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		for i, outpoint := range outpoints {
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

	return unspent, nil
}

func (r *RedisStore) SearchMembers(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	if items, err := r.Search(ctx, cfg); err != nil {
		return nil, err
	} else {
		members := make([]string, 0, len(items))
		for _, item := range items {
			members = append(members, item.Member)
		}
		return members, nil
	}
}

func (r *RedisStore) SearchOutpoints(ctx context.Context, cfg *idx.SearchCfg) (results []string, err error) {
	cfg.OutpointsOnly = true
	if items, err := r.Search(ctx, cfg); err != nil {
		return nil, err
	} else {
		members := make([]string, 0, len(items))
		for _, item := range items {
			members = append(members, item.Member)
		}
		return members, nil
	}
}

func (r *RedisStore) SearchTxos(ctx context.Context, cfg *idx.SearchCfg) (txos []*idx.Txo, err error) {
	results, err := r.Search(ctx, cfg)
	if err != nil {
		return nil, err
	}
	outpoints := make([]string, 0, len(results))
	resultMap := make(map[string]*idx.Log, len(results))
	for _, result := range results {
		resultMap[result.Member] = result
		outpoints = append(outpoints, result.Member)
	}
	if cfg.FilterSpent {
		if outpoints, err = r.filterSpent(ctx, outpoints, cfg.RefreshSpends); err != nil {
			return nil, err
		}
	}

	if cfg.IncludeTxo {
		if txos, err = r.LoadTxos(ctx, outpoints, cfg.IncludeTags, cfg.IncludeScript); err != nil {
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
			} else if txo.Data, err = r.LoadData(ctx, txo.Outpoint.String(), cfg.IncludeTags); err != nil {
				return nil, err
			} else if txo.Data == nil {
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

func (r *RedisStore) SearchBalance(ctx context.Context, cfg *idx.SearchCfg) (balance uint64, err error) {
	cfg.OutpointsOnly = true
	if outpoints, err := r.SearchMembers(ctx, cfg); err != nil {
		return 0, err
	} else if outpoints, err = r.filterSpent(ctx, outpoints, cfg.RefreshSpends); err != nil {
		return 0, err
	} else if txos, err := r.LoadTxos(ctx, outpoints, nil, false); err != nil {
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

func (r *RedisStore) SearchTxns(ctx context.Context, cfg *idx.SearchCfg) (txns []*lib.TxResult, err error) {
	txMap := make(map[float64]*lib.TxResult)
	scores := make([]float64, 0, 1000)
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
			}
			if out != nil {
				result.Outputs[*out] = struct{}{}
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
	} else if outpoints, err = r.SearchMembers(ctx, &idx.SearchCfg{
		Keys:          []string{key},
		OutpointsOnly: true,
	}); err != nil {
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
