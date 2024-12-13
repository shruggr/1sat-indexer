package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/vmihailenco/msgpack/v5"
)

type RedisStore struct {
	DB *redis.Client
}

func NewRedisStore(connString string) (*RedisStore, error) {
	r := &RedisStore{}
	if opts, err := redis.ParseURL(connString); err != nil {
		return nil, err
	} else {
		r.DB = redis.NewClient(opts)
		return r, nil
	}
}

func (r *RedisStore) LoadTxo(ctx context.Context, outpoint string, tags []string) (*idx.Txo, error) {
	if result, err := r.DB.HGet(ctx, TxosKey, outpoint).Bytes(); err == redis.Nil {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else {
		txo := &idx.Txo{}
		if err := msgpack.Unmarshal(result, txo); err != nil {
			log.Panic(err)
			return nil, err
		}
		txo.Score = idx.HeightScore(txo.Height, txo.Idx)
		if txo.Data, err = r.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
			return nil, err
		} else if txo.Data == nil {
			txo.Data = make(idx.IndexDataMap)
		}
		return txo, nil
	}
}

func (r *RedisStore) LoadTxos(ctx context.Context, outpoints []string, tags []string) ([]*idx.Txo, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}
	if msgpacks, err := r.DB.HMGet(ctx, TxosKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		txos := make([]*idx.Txo, 0, len(msgpacks))
		for _, mp := range msgpacks {
			var txo *idx.Txo
			if mp != nil {
				// outpoint := outpoints[i]
				txo = &idx.Txo{}
				if err = msgpack.Unmarshal([]byte(mp.(string)), txo); err != nil {
					return nil, err
				}
				txo.Score = idx.HeightScore(txo.Height, txo.Idx)
				if txo.Data, err = r.LoadData(ctx, txo.Outpoint.String(), tags); err != nil {
					return nil, err
				} else if txo.Data == nil {
					txo.Data = make(idx.IndexDataMap)
				}
			}
			txos = append(txos, txo)
		}
		return txos, nil
	}
}

func (r *RedisStore) LoadTxosByTxid(ctx context.Context, txid string, tags []string) ([]*idx.Txo, error) {
	// Get all keys matching the pattern txid_*
	pattern := fmt.Sprintf("%s_*", txid)
	outpoints := make([]string, 0)
	
	// Use HSCAN to find all matching outpoints
	iter := r.DB.HScan(ctx, TxosKey, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		// Skip every other value since HSCAN returns key-value pairs
		outpoint := iter.Val()
		if iter.Next(ctx) {
			outpoints = append(outpoints, outpoint)
		}
	}
	if err := iter.Err(); err != nil {
		log.Panic(err)
		return nil, err
	}

	// Use LoadTxos to load the full txo data for matching outpoints
	return r.LoadTxos(ctx, outpoints, tags)
}

func (r *RedisStore) LoadData(ctx context.Context, outpoint string, tags []string) (data idx.IndexDataMap, err error) {
	if len(tags) == 0 {
		return nil, nil
	}
	data = make(idx.IndexDataMap, len(tags))
	if datas, err := r.DB.HMGet(ctx, TxoDataKey(outpoint), tags...).Result(); err != nil {
		log.Panic(err)
		return nil, err
	} else {
		for i, tag := range tags {
			if datas[i] != nil {
				data[tag] = &idx.IndexData{
					Data: json.RawMessage(datas[i].(string)),
				}
			}
		}
	}
	return
}

func (r *RedisStore) SaveTxo(ctx context.Context, txo *idx.Txo, height uint32, blkIdx uint64) error {
	outpoint := txo.Outpoint.String()
	score := idx.HeightScore(height, blkIdx)

	accounts, err := r.AcctsByOwners(ctx, txo.Owners)
	if err != nil {
		log.Panic(err)
		return err
	}
	if mp, err := msgpack.Marshal(txo); err != nil {
		log.Println("Marshal Txo", err)
		log.Panic(err)
		return err
	} else if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) (err error) {
		if err := pipe.HSet(ctx, TxosKey, outpoint, mp).Err(); err != nil {
			log.Println("HSET Txo", err)
			log.Panic(err)
			return err
		}
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			if err := pipe.ZAdd(ctx, idx.OwnerTxosKey(owner), redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Owner", err)
				log.Panic(err)
				return err
			}
		}
		for _, acct := range accounts {
			if err := pipe.ZAdd(ctx, idx.AccountTxosKey(acct), redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Account", err)
				log.Panic(err)
				return err
			}
		}

		return nil
	}); err != nil {
		log.Panic(err)
		return err
	} else if err = r.SaveTxoData(ctx, txo); err != nil {
		log.Panic(err)
		return err
	}

	for _, owner := range txo.Owners {
		evt.Publish(ctx, idx.OwnerKey(owner), outpoint)
	}
	for _, account := range accounts {
		evt.Publish(ctx, idx.AccountKey(account), outpoint)
	}

	return nil
}

func (r *RedisStore) SaveSpend(ctx context.Context, spend *idx.Txo, txid string, height uint32, blkIdx uint64) error {
	score := idx.HeightScore(height, blkIdx)
	if accounts, err := r.AcctsByOwners(ctx, spend.Owners); err != nil {
		log.Panic(err)
		return err
	} else if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.HSet(ctx,
			SpendsKey,
			spend.Outpoint.String(),
			txid,
		).Err(); err != nil {
			return err
		}
		for _, owner := range spend.Owners {
			if owner == "" {
				continue
			}
			if err := pipe.ZAdd(ctx, idx.OwnerTxosKey(owner), redis.Z{
				Score:  score,
				Member: txid,
			}).Err(); err != nil {
				return err
			}
		}
		for _, account := range accounts {
			if err := pipe.ZAdd(ctx, idx.AccountTxosKey(account), redis.Z{
				Score:  score,
				Member: txid,
			}).Err(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}

	// for _, owner := range spend.Owners {
	// 	evt.Publish(ctx, idx.OwnerKey(owner), txid)
	// }
	// for _, account := range accounts {
	// 	evt.Publish(ctx, idx.AccountKey(account), txid)
	// }

	return nil
}

func (r *RedisStore) RollbackSpend(ctx context.Context, spend *idx.Txo, txid string) error {
	if accounts, err := r.AcctsByOwners(ctx, spend.Owners); err != nil {
		log.Panic(err)
		return err
	} else if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.HDel(ctx,
			SpendsKey,
			spend.Outpoint.String(),
		).Err(); err != nil {
			return err
		}
		for _, owner := range spend.Owners {
			if owner == "" {
				continue
			} else if err := pipe.ZRem(ctx, idx.OwnerTxosKey(owner), txid).Err(); err != nil {
				return err
			}
		}
		for _, account := range accounts {
			if err := pipe.ZRem(ctx, idx.AccountTxosKey(account), txid).Err(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func (r *RedisStore) SaveTxoData(ctx context.Context, txo *idx.Txo) (err error) {
	if len(txo.Data) == 0 {
		return nil
	}

	outpoint := txo.Outpoint.String()
	score := idx.HeightScore(txo.Height, txo.Idx)
	datas := make(map[string]any, len(txo.Data))
	if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for tag, data := range txo.Data {
			tagKey := evt.TagKey(tag)
			if err := pipe.ZAdd(ctx, tagKey, redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Tag", tagKey, err)
				log.Panic(err)
				return err
			}
			for _, event := range data.Events {
				eventKey := evt.EventKey(tag, event)
				if err := pipe.ZAdd(ctx, eventKey, redis.Z{
					Score:  score,
					Member: outpoint,
				}).Err(); err != nil {
					log.Panic(err)
					log.Println("ZADD Event", eventKey, err)
					return err
				}
			}
			if datas[tag], err = data.MarshalJSON(); err != nil {
				log.Panic(err)
				return err
			}
		}
		if len(datas) > 0 {
			if err := pipe.HSet(ctx, TxoDataKey(outpoint), datas).Err(); err != nil {
				log.Panic(err)
				log.Println("HSET TxoData", err)
				return err
			}
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}
	for tag, data := range txo.Data {
		for _, event := range data.Events {
			evt.Publish(ctx, evt.EventKey(tag, event), outpoint)
		}
	}
	return nil
}

func (r *RedisStore) GetSpend(ctx context.Context, outpoint string) (string, error) {
	return r.DB.HGet(ctx, SpendsKey, outpoint).Result()
}

func (r *RedisStore) GetSpends(ctx context.Context, outpoints []string) ([]string, error) {
	spends := make([]string, 0, len(outpoints))
	if records, err := r.DB.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		for _, record := range records {
			if record != nil {
				spends = append(spends, record.(string))
			}
		}
	}
	return spends, nil
}

func (r *RedisStore) SetNewSpend(ctx context.Context, outpoint, txid string) (bool, error) {
	return r.DB.HSetNX(ctx, SpendsKey, outpoint, txid).Result()
}

func (r *RedisStore) UnsetSpends(ctx context.Context, outpoints []string) error {
	return r.DB.HDel(ctx, SpendsKey, outpoints...).Err()
}

func (r *RedisStore) RollbackTxo(ctx context.Context, txo *idx.Txo) error {
	outpoint := txo.Outpoint.String()
	accounts, err := r.AcctsByOwners(ctx, txo.Owners)
	if err != nil {
		log.Panic(err)
		return err
	}

	if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) (err error) {
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			if err := pipe.ZRem(ctx, idx.OwnerTxosKey(owner), outpoint).Err(); err != nil {
				log.Println("ZRem Owner", err)
				log.Panic(err)
				return err
			}
		}
		for _, acct := range accounts {
			if err := pipe.ZRem(ctx, idx.AccountTxosKey(acct), outpoint).Err(); err != nil {
				log.Println("ZRem Account", err)
				log.Panic(err)
				return err
			}
		}
		for tag, data := range txo.Data {
			tagKey := evt.TagKey(tag)
			if err := pipe.ZRem(ctx, tagKey, outpoint).Err(); err != nil {
				log.Println("ZRem Tag", tagKey, err)
				log.Panic(err)
				return err
			}
			for _, event := range data.Events {
				eventKey := evt.EventKey(tag, event)
				if err := pipe.ZRem(ctx, eventKey, outpoint).Err(); err != nil {
					log.Panic(err)
					log.Println("ZADD Event", eventKey, err)
					return err
				}
			}
		}

		if err := pipe.HDel(ctx, TxosKey, outpoint).Err(); err != nil {
			log.Println("HDEL Txo", err)
			log.Panic(err)
			return err
		} else if err := pipe.Del(ctx, TxoDataKey(outpoint)).Err(); err != nil {
			log.Println("HDEL TxoData", err)
			log.Panic(err)
			return err
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}
	return nil
}
