package redisstore

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func (r *RedisStore) Delog(ctx context.Context, key string, members ...string) error {
	memInt := make([]interface{}, len(members))
	for i, m := range members {
		memInt[i] = m
	}
	return r.DB.ZRem(ctx, key, memInt...).Err()
}

func (r *RedisStore) Log(ctx context.Context, key string, member string, score float64) (err error) {
	return r.DB.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: member,
	}).Err()
}

func (r *RedisStore) LogOnce(ctx context.Context, key string, member string, score float64) (bool, error) {
	if rows, err := r.DB.ZAddNX(ctx, key, redis.Z{
		Score:  score,
		Member: member,
	}).Result(); err != nil {
		return false, err
	} else {
		return rows > 0, nil
	}
}

func (r *RedisStore) LogScore(ctx context.Context, key string, member string) (score float64, err error) {
	if score, err = r.DB.ZScore(ctx, key, member).Result(); err == redis.Nil {
		err = nil
	}
	return
}
