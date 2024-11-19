package redisstore

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func (r *RedisStore) Delog(ctx context.Context, tag string, id string) error {
	return r.QueueDB.ZRem(ctx, LogKey(tag), id).Err()
}

func (r *RedisStore) Log(ctx context.Context, tag string, id string, score float64) (err error) {
	return r.QueueDB.ZAdd(ctx, LogKey(tag), redis.Z{
		Score:  score,
		Member: id,
	}).Err()
}

func (r *RedisStore) LogScore(ctx context.Context, tag string, id string) (score float64, err error) {
	if score, err = r.QueueDB.ZScore(ctx, LogKey(tag), id).Result(); err == redis.Nil {
		err = nil
	}
	return
}
