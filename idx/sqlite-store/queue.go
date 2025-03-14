package sqlitestore

import (
	"context"
	"database/sql"

	"github.com/shruggr/1sat-indexer/v5/idx"
)

func (s *SQLiteStore) Delog(ctx context.Context, key string, members ...string) error {
	query := `DELETE FROM logs WHERE search_key = ? AND member IN (` + placeholders(len(members)) + `)`
	args := append([]interface{}{key}, toInterfaceSlice(members)...)
	_, err := s.DB.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLiteStore) Log(ctx context.Context, key string, member string, score float64) (err error) {
	_, err = insLog.ExecContext(ctx, key, member, score, score)
	// _, err = s.DB.ExecContext(ctx, `INSERT INTO logs(search_key, member, score)
	//     VALUES (?, ?, ?)
	//     ON CONFLICT (search_key, member) DO UPDATE SET score = ?`,
	// 	key,
	// 	member,
	// 	score,
	// 	score,
	// )
	return
}

func (s *SQLiteStore) LogMany(ctx context.Context, key string, logs []idx.Log) error {
	for _, l := range logs {
		if _, err := insLog.ExecContext(ctx, key, l.Member, l.Score, l.Score); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) LogOnce(ctx context.Context, key string, member string, score float64) (bool, error) {
	result, err := putLogOnce.ExecContext(ctx, key, member, score)
	// result, err := s.DB.ExecContext(ctx, `INSERT INTO logs(search_key, member, score)
	//     VALUES (?, ?, ?)
	//     ON CONFLICT DO NOTHING`,
	// 	key,
	// 	member,
	// 	score,
	// )
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
	// err = s.DB.QueryRowContext(ctx, `SELECT score FROM logs
	//     WHERE search_key = ? AND member = ?`,
	// 	key,
	// 	member,
	// ).Scan(&score)
	if err == sql.ErrNoRows {
		err = nil
	}
	return
}
