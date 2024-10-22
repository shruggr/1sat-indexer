package idx

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/lib"
)

type SearchCfg struct {
	Tag     string
	Event   *evt.Event
	From    *float64
	Limit   *uint64
	Reverse bool
}

type SearchResult struct {
	Outpoint *lib.Outpoint
	Score    float64
}

func Search(ctx context.Context, cfg *SearchCfg) (results []*SearchResult, err error) {
	query := redis.ZRangeBy{}
	if cfg.Limit != nil {
		query.Count = int64(*cfg.Limit)
	}
	var key string
	if cfg.Event == nil {
		key = evt.TagKey(cfg.Tag)
	} else {
		key = evt.EventKey(cfg.Tag, cfg.Event)
	}
	var items []redis.Z
	if cfg.Reverse {
		if cfg.From != nil {
			query.Max = fmt.Sprintf("(%f", *cfg.From)
		} else {
			query.Max = "+inf"
		}
		query.Min = "-inf"
		if items, err = TxoDB.ZRevRangeByScoreWithScores(ctx, key, &query).Result(); err != nil {
			return nil, err
		}
	} else {
		if cfg.From != nil {
			query.Min = fmt.Sprintf("(%f", *cfg.From)
		} else {
			query.Min = "-inf"
		}
		query.Max = "+inf"
		if items, err = TxoDB.ZRevRangeByScoreWithScores(ctx, key, &query).Result(); err != nil {
			return nil, err
		}
	}

	out := make([]*SearchResult, 0, len(results))
	for _, item := range items {
		result := &SearchResult{
			Score: item.Score,
		}
		if result.Outpoint, err = lib.NewOutpointFromString(item.Member.(string)); err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}
