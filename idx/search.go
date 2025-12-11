package idx

import (
	"context"

	"github.com/b-open-io/overlay/queue"
)

const pageSize = int64(100)

type ComparisonType int

var (
	ComparisonOR  = ComparisonType(0)
	ComparisonAND = ComparisonType(1)
)

type SearchCfg struct {
	Keys           []string
	From           *float64
	To             *float64
	Limit          uint32
	ComparisonType ComparisonType
	Reverse        bool
	ExcludeMined   bool
	ExcludeMempool bool
	IncludeTxo     bool
	IncludeScript  bool
	IncludeTags    []string
	IncludeRawtx   bool
	OutpointsOnly  bool
	FilterSpent    bool
}

type cursor struct {
	key     string
	results []queue.ScoredMember
	pos     int
	offset  int64
	done    bool
}

func (s *QueueStore) Search(ctx context.Context, cfg *SearchCfg) (records []queue.ScoredMember, err error) {
	if len(cfg.Keys) == 0 {
		return nil, nil
	}

	limit := int(cfg.Limit)
	if limit == 0 {
		limit = 1000
	}

	cursors := make([]*cursor, len(cfg.Keys))
	for i, key := range cfg.Keys {
		cursors[i] = &cursor{key: key}
		if err := s.fetchPage(ctx, cursors[i], cfg); err != nil {
			return nil, err
		}
	}

	seen := make(map[string]struct{})
	records = make([]queue.ScoredMember, 0, limit)
	results := make([]queue.ScoredMember, 0, pageSize)

	for len(records) < limit {
		results = results[:0]

		for len(results) < int(pageSize) {
			member, score, ok, err := s.nextResult(ctx, cursors, cfg, seen)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			seen[member] = struct{}{}
			results = append(results, queue.ScoredMember{Member: member, Score: score})
		}

		if len(results) == 0 {
			break
		}

		if cfg.FilterSpent {
			results, err = s.filterSpent(ctx, results)
			if err != nil {
				return nil, err
			}
		}
		for _, p := range results {
			if len(records) >= limit {
				break
			}
			records = append(records, p)
		}
	}

	return records, nil
}

func (s *QueueStore) nextResult(ctx context.Context, cursors []*cursor, cfg *SearchCfg, seen map[string]struct{}) (string, float64, bool, error) {
	for {
		var best *cursor
		var bestScore float64
		var bestMember string

		for _, c := range cursors {
			if c.pos >= len(c.results) {
				if c.done {
					continue
				}
				if err := s.fetchPage(ctx, c, cfg); err != nil {
					return "", 0, false, err
				}
				if c.pos >= len(c.results) {
					continue
				}
			}
			m := c.results[c.pos]
			if best == nil ||
				(cfg.Reverse && m.Score > bestScore) ||
				(!cfg.Reverse && m.Score < bestScore) {
				best = c
				bestScore = m.Score
				bestMember = m.Member
			}
		}

		if best == nil {
			return "", 0, false, nil
		}

		if cfg.ComparisonType == ComparisonAND {
			if !s.inAllCursors(cursors, bestMember) {
				for _, c := range cursors {
					if c.pos < len(c.results) && c.results[c.pos].Member == bestMember {
						c.pos++
					}
				}
				continue
			}
			for _, c := range cursors {
				c.pos++
			}
		} else {
			best.pos++
		}

		if _, ok := seen[bestMember]; !ok {
			return bestMember, bestScore, true, nil
		}
	}
}

func (s *QueueStore) inAllCursors(cursors []*cursor, member string) bool {
	for _, c := range cursors {
		if c.pos >= len(c.results) || c.results[c.pos].Member != member {
			return false
		}
	}
	return true
}

func (s *QueueStore) filterSpent(ctx context.Context, pending []queue.ScoredMember) ([]queue.ScoredMember, error) {
	outpoints := make([]string, len(pending))
	for i, p := range pending {
		outpoints[i] = p.Member
	}

	spends, err := s.GetSpends(ctx, outpoints)
	if err != nil {
		return nil, err
	}

	result := make([]queue.ScoredMember, 0, len(pending))
	for i, p := range pending {
		if spends[i] == "" {
			result = append(result, p)
		}
	}
	return result, nil
}

func (s *QueueStore) fetchPage(ctx context.Context, c *cursor, cfg *SearchCfg) error {
	scoreRange := queue.ScoreRange{
		Min:    cfg.From,
		Max:    cfg.To,
		Offset: c.offset,
		Count:  pageSize,
	}

	var err error
	if cfg.Reverse {
		c.results, err = s.Queue.ZRevRange(ctx, c.key, scoreRange)
	} else {
		c.results, err = s.Queue.ZRange(ctx, c.key, scoreRange)
	}
	if err != nil {
		return err
	}

	c.pos = 0
	c.offset += int64(len(c.results))
	if len(c.results) < int(pageSize) {
		c.done = true
	}
	return nil
}

func (s *QueueStore) SearchTxos(ctx context.Context, cfg *SearchCfg) ([]*Txo, error) {
	if cfg.OutpointsOnly {
		return nil, nil
	}

	logs, err := s.Search(ctx, cfg)
	if err != nil {
		return nil, err
	}

	outpoints := make([]string, len(logs))
	for i, log := range logs {
		outpoints[i] = log.Member
	}

	return s.LoadTxos(ctx, outpoints, cfg.IncludeTags, false)
}

func (s *QueueStore) SearchBalance(ctx context.Context, cfg *SearchCfg) (uint64, error) {
	cfg.FilterSpent = true
	logs, err := s.Search(ctx, cfg)
	if err != nil {
		return 0, err
	}

	if len(logs) == 0 {
		return 0, nil
	}

	outpoints := make([]string, len(logs))
	for i, log := range logs {
		outpoints[i] = log.Member
	}

	sats, err := s.GetSats(ctx, outpoints)
	if err != nil {
		return 0, err
	}

	var balance uint64
	for _, s := range sats {
		balance += s
	}
	return balance, nil
}

func (s *QueueStore) CountMembers(ctx context.Context, key string) (uint64, error) {
	count, err := s.Queue.ZCard(ctx, key)
	if err != nil {
		return 0, err
	}
	return uint64(count), nil
}
