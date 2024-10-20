package evt

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

type SearchCfg struct {
	Tag     string
	Event   *Event
	From    *float64
	Limit   uint64
	Reverse bool
}

type SearchResult struct {
	Outpoint *lib.Outpoint
	Score    float64
}

func Search(ctx context.Context, cfg *SearchCfg) ([]string, error) {
	query := &redis.ZRangeArgs{
		ByScore: true,
		Count:   int64(cfg.Limit),
	}
	if cfg.Event == nil {
		query.Key = TagKey(cfg.Tag)
	} else {
		query.Key = EventKey(cfg.Tag, cfg.Event)
	}
	if cfg.From != nil {
		query.Start = fmt.Sprintf("(%f", cfg.From)
	} else {
		query.Start = "+inf"
	}
	if cfg.Reverse {
		query.Rev = true
		query.Stop = "-inf"
	} else {
		query.Stop = "+inf"
	}

	if results, err := DB.ZRangeArgsWithScores(ctx, *query).Result(); err != nil {
		return nil, err
	} else {
		out := make([]string, 0, len(results))
		for _, item := range results {
			out = append(out, item.Member.(string))
		}
		return out, nil
	}
}
