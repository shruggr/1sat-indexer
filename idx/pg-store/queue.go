package pgstore

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (p *PGStore) Delog(ctx context.Context, tag string, member string) error {
	_, err := p.DB.Exec(ctx, `DELETE FROM logs
		WHERE search_key=$1 AND member=$2`,
		tag,
		member,
	)
	return err
}

func (p *PGStore) Log(ctx context.Context, tag string, member string, score float64) (err error) {
	_, err = p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
		VALUES ($1, $2, $3)`,
		tag,
		member,
		score,
	)
	return
}

func (p *PGStore) LogScore(ctx context.Context, tag string, member string) (score float64, err error) {
	err = p.DB.QueryRow(ctx, `SELECT score FROM logs
		WHERE search_key=$1 AND member=$2`,
		tag,
		member,
	).Scan(&score)
	if err == pgx.ErrNoRows {
		err = nil
	}
	return
}
