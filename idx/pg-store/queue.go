package pgstore

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (p *PGStore) Delog(ctx context.Context, key string, members ...string) error {
	_, err := p.DB.Exec(ctx, `DELETE FROM logs
		WHERE search_key=$1 AND member=ANY($2)`,
		key,
		members,
	)
	return err
}

func (p *PGStore) Log(ctx context.Context, key string, member string, score float64) (err error) {
	_, err = p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
		VALUES ($1, $2, $3)
		ON CONFLICT (search_key, member) DO UPDATE SET score = $3`,
		key,
		member,
		score,
	)
	return
}

func (p *PGStore) LogOnce(ctx context.Context, key string, member string, score float64) (bool, error) {
	if result, err := p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`,
		key,
		member,
		score,
	); err != nil {
		return false, err
	} else {
		return result.RowsAffected() > 0, nil
	}
}

func (p *PGStore) LogScore(ctx context.Context, key string, member string) (score float64, err error) {
	err = p.DB.QueryRow(ctx, `SELECT score FROM logs
		WHERE search_key=$1 AND member=$2`,
		key,
		member,
	).Scan(&score)
	if err == pgx.ErrNoRows {
		err = nil
	}
	return
}
