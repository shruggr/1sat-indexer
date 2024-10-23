package idx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

type SearchCfg struct {
	Key     string
	From    *float64
	Limit   uint32
	Reverse bool
}

type SearchResult struct {
	Outpoint string
	Score    float64
}

func BuildQuery(cfg *SearchCfg) *redis.ZRangeBy {
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
	return query
}

func Search(ctx context.Context, cfg *SearchCfg) (results []string, err error) {
	query := BuildQuery(cfg)
	var items []redis.Z
	if cfg.Reverse {
		items, err = TxoDB.ZRevRangeByScoreWithScores(ctx, cfg.Key, query).Result()
	} else {
		items, err = TxoDB.ZRevRangeByScoreWithScores(ctx, cfg.Key, query).Result()
	}
	if err != nil {
		return nil, err
	}
	outpoints := make([]string, 0, len(items))
	for _, item := range items {
		outpoints = append(outpoints, item.Member.(string))
	}
	return outpoints, nil
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
			if spends[i] == nil {
				unspent = append(unspent, outpoint)
			}
		}
		return unspent, nil
	}
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
