package sqlitestore

import (
	"context"
	"database/sql"

	"github.com/shruggr/1sat-indexer/v5/idx"
)

func (s *SQLiteStore) Delog(ctx context.Context, key string, members ...string) error {
	for _, member := range members {
		if _, err := s.execute(ctx, delLog, key, member); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) Log(ctx context.Context, key string, member string, score float64) (err error) {
	_, err = s.execute(ctx, insLog, key, member, score, score)
	return
}

func (s *SQLiteStore) LogMany(ctx context.Context, key string, logs []idx.Log) error {
	for _, l := range logs {
		if _, err := s.execute(ctx, insLog, key, l.Member, l.Score, l.Score); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) LogOnce(ctx context.Context, key string, member string, score float64) (bool, error) {
	result, err := s.execute(ctx, putLogOnce, key, member, score)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (s *SQLiteStore) LogScore(ctx context.Context, key string, member string) (score float64, err error) {
	err = getLogScore.QueryRowContext(ctx, key, member).Scan(&score)
	if err == sql.ErrNoRows {
		err = nil
	}
	return
}
