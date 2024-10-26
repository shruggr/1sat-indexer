package idx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/vmihailenco/msgpack/v5"
)

type SearchCfg struct {
	Key           string
	From          float64
	Limit         uint32
	Reverse       bool
	IncludeTxo    bool
	IncludeScript bool
	IncludeTags   []string
}

type SearchResult struct {
	Outpoint string
	Score    float64
}

func BuildQuery(cfg *SearchCfg) *redis.ZRangeBy {
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

func SearchWithScores(ctx context.Context, cfg *SearchCfg) (results []redis.Z, err error) {
	query := BuildQuery(cfg)
	if cfg.Reverse {
		return TxoDB.ZRevRangeByScoreWithScores(ctx, cfg.Key, query).Result()
	} else {
		return TxoDB.ZRangeByScoreWithScores(ctx, cfg.Key, query).Result()
	}
}

func Search(ctx context.Context, cfg *SearchCfg) (results []string, err error) {
	if items, err := SearchWithScores(ctx, cfg); err != nil {
		return nil, err
	} else {
		outpoints := make([]string, 0, len(items))
		for _, item := range items {
			outpoints = append(outpoints, item.Member.(string))
		}
		return outpoints, nil
	}
}

func FilterSpent(ctx context.Context, outpoints []string) ([]string, error) {
	if len(outpoints) == 0 {
		return outpoints, nil
	}
	if spends, err := TxoDB.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		unspent := make([]string, 0, len(outpoints))
		for i, outpoint := range outpoints {
			if len(outpoint) < 65 {
				continue
			}
			if spends[i] == nil {
				unspent = append(unspent, outpoint)
			}
		}
		return unspent, nil
	}
}

func SearchTxos(ctx context.Context, cfg *SearchCfg) (txos []*Txo, err error) {
	if cfg.IncludeTxo {
		if outpoints, err := Search(ctx, cfg); err != nil {
			return nil, err
		} else if txos, err = LoadTxos(ctx, outpoints, nil); err != nil {
			return nil, err
		}
	} else {
		if results, err := SearchWithScores(ctx, cfg); err != nil {
			return nil, err
		} else {
			txos = make([]*Txo, 0, len(results))
			for _, result := range results {
				txo := &Txo{
					Height: uint32(result.Score / 1000000000),
					Idx:    uint64(result.Score) % 1000000000,
					Score:  result.Score,
					Data:   make(map[string]*IndexData),
				}
				if txo.Outpoint, err = lib.NewOutpointFromString(result.Member.(string)); err != nil {
					return nil, err
				}
				txos = append(txos, txo)
			}
		}
	}
	if len(cfg.IncludeTags) > 0 {
		for _, txo := range txos {
			if err := txo.LoadData(ctx, cfg.IncludeTags); err != nil {
				return nil, err
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

func SearchTxns(ctx context.Context, cfg *SearchCfg) (txns []*lib.TxResult, err error) {
	results := make([]*lib.TxResult, 0, 1000)
	txMap := make(map[float64]*lib.TxResult)
	if activity, err := SearchWithScores(ctx, cfg); err != nil {
		return nil, err
	} else {
		for _, item := range activity {
			var txid string
			var out *uint32
			member := item.Member.(string)
			if len(member) == 64 {
				txid = member
			} else if outpoint, err := lib.NewOutpointFromString(member); err != nil {
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

func SearchUtxos(ctx context.Context, cfg *SearchCfg) ([]string, error) {
	if results, err := Search(ctx, cfg); err != nil {
		return nil, err
	} else if results, err = FilterSpent(ctx, results); err != nil {
		return nil, err
	} else {
		return results, nil
	}
}

func Balance(ctx context.Context, key string) (balance uint64, err error) {
	var outpoints []string
	balanceKey := BalanceKey(key)
	if balance, err = AcctDB.Get(ctx, balanceKey).Uint64(); err != nil && err != redis.Nil {
		return 0, err
	} else if err != redis.Nil {
		return balance, nil
	} else if outpoints, err = Search(ctx, &SearchCfg{Key: key}); err != nil {
		return 0, err
	}
	var msgpacks []interface{}
	if msgpacks, err = TxoDB.HMGet(ctx, TxosKey, outpoints...).Result(); err != nil {
		return
	}
	for _, mp := range msgpacks {
		if mp != nil {
			var txo Txo
			if err = msgpack.Unmarshal([]byte(mp.(string)), &txo); err != nil {
				return
			}
			if txo.Satoshis != nil {
				balance += *txo.Satoshis
			}
		}
	}
	err = AcctDB.Set(ctx, balanceKey, balance, 60*time.Second).Err()
	return
}
