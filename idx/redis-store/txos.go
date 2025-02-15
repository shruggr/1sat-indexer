package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
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

func (r *RedisStore) LoadTxo(ctx context.Context, outpoint string, tags []string, script bool, spend bool) (*idx.Txo, error) {
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
		if spend {
			if txo.Spend, err = r.GetSpend(ctx, outpoint, false); err != nil {
				return nil, err
			}
		}
		if script {
			if err = txo.LoadScript(ctx); err != nil {
				return nil, err
			}
		}
		return txo, nil
	}
}

func (r *RedisStore) LoadTxos(ctx context.Context, outpoints []string, tags []string, script bool, spend bool) ([]*idx.Txo, error) {
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
				if script {
					if err = txo.LoadScript(ctx); err != nil {
						return nil, err
					}
				}
			}
			txos = append(txos, txo)
		}
		if spend {
			if spends, err := r.GetSpends(ctx, outpoints, false); err != nil {
				return nil, err
			} else {
				for i, spend := range spends {
					txos[i].Spend = spend
				}
			}
		}
		return txos, nil
	}
}

func (r *RedisStore) LoadTxosByTxid(ctx context.Context, txid string, tags []string, script bool, spend bool) ([]*idx.Txo, error) {
	pattern := fmt.Sprintf("%s_*", txid)
	outpoints := make([]string, 0)

	iter := r.DB.HScan(ctx, TxosKey, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		outpoint := iter.Val()
		if iter.Next(ctx) {
			outpoints = append(outpoints, outpoint)
		}
	}
	if err := iter.Err(); err != nil {
		log.Panic(err)
		return nil, err
	}

	return r.LoadTxos(ctx, outpoints, tags, script, spend)
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

func (r *RedisStore) SaveTxos(idxCtx *idx.IndexContext) (err error) {
	for _, txo := range idxCtx.Txos {
		if err := r.saveTxo(idxCtx.Ctx, txo, idxCtx.Height, idxCtx.Idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisStore) saveTxo(ctx context.Context, txo *idx.Txo, height uint32, blkIdx uint64) (err error) {
	outpoint := txo.Outpoint.String()
	score := idx.HeightScore(height, blkIdx)

	txo.Events = make([]string, 0, 100)
	datas := make(map[string]any, len(txo.Data))
	for tag, data := range txo.Data {
		txo.Events = append(txo.Events, evt.TagKey(tag))
		for _, event := range data.Events {
			txo.Events = append(txo.Events, evt.EventKey(tag, event))
		}
		if datas[tag], err = data.MarshalJSON(); err != nil {
			log.Panic(err)
			return err
		}
	}
	for _, owner := range txo.Owners {
		if owner == "" {
			continue
		}
		txo.Events = append(txo.Events, idx.OwnerKey(owner))
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

		if len(datas) > 0 {
			if err := pipe.HSet(ctx, TxoDataKey(outpoint), datas).Err(); err != nil {
				log.Panic(err)
				log.Println("HSET TxoData", err)
				return err
			}
		}
		for _, event := range txo.Events {
			if err := pipe.ZAdd(ctx, event, redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Event", event, err)
				log.Panic(err)
				return err
			}
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}

	for _, event := range txo.Events {
		evt.Publish(ctx, event, outpoint)
	}

	return nil
}

func (r *RedisStore) SaveSpends(idxCtx *idx.IndexContext) (err error) {
	score := idx.HeightScore(idxCtx.Height, idxCtx.Idx)
	if _, err := r.DB.Pipelined(idxCtx.Ctx, func(pipe redis.Pipeliner) error {
		spends := make(map[string]string, len(idxCtx.Spends))
		outpoints := make([]interface{}, 0, len(idxCtx.Spends))
		owners := make(map[string]struct{}, 10)
		for _, spend := range idxCtx.Spends {
			outpoint := spend.Outpoint.String()
			spends[outpoint] = idxCtx.TxidHex
			outpoints = append(outpoints, outpoint)
			for _, owner := range spend.Owners {
				if owner == "" {
					continue
				}
				owners[owner] = struct{}{}
			}
		}
		if err := pipe.HMSet(idxCtx.Ctx, SpendsKey, spends).Err(); err != nil {
			return err
		} else if err := pipe.SAdd(idxCtx.Ctx, InputsKey(idxCtx.TxidHex), outpoints...).Err(); err != nil {
			return err
		}

		for owner, _ := range owners {
			if err := pipe.ZAdd(idxCtx.Ctx, idx.OwnerKey(owner), redis.Z{
				Score:  score,
				Member: idxCtx.TxidHex,
			}).Err(); err != nil {
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

// func (r *RedisStore) SaveSpend(ctx context.Context, spend *idx.Txo, txid string, height uint32, blkIdx uint64) error {
// 	score := idx.HeightScore(height, blkIdx)
// 	if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
// 		if err := pipe.HSet(ctx,
// 			SpendsKey,
// 			spend.Outpoint.String(),
// 			txid,
// 		).Err(); err != nil {
// 			return err
// 		}
// 		for _, owner := range spend.Owners {
// 			if owner == "" {
// 				continue
// 			}
// 			if err := pipe.ZAdd(ctx, idx.OwnerKey(owner), redis.Z{
// 				Score:  score,
// 				Member: txid,
// 			}).Err(); err != nil {
// 				return err
// 			}
// 		}
// 		return nil
// 	}); err != nil {
// 		log.Panic(err)
// 		return err
// 	}

// 	return nil
// }

// func (r *RedisStore) RollbackSpend(ctx context.Context, spend *idx.Txo, txid string) (err error) {
// 	prevSpend := ""
// 	if prevSpend, err = r.GetSpend(ctx, spend.Outpoint.String(), false); err != nil {
// 		log.Panic(err)
// 		return err
// 	}

// 	if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
// 		if prevSpend == txid {
// 			if err := pipe.HDel(ctx,
// 				SpendsKey,
// 				spend.Outpoint.String(),
// 			).Err(); err != nil {
// 				return err
// 			}
// 		}
// 		for _, owner := range spend.Owners {
// 			if owner == "" {
// 				continue
// 			} else if err := pipe.ZRem(ctx, idx.OwnerKey(owner), txid).Err(); err != nil {
// 				return err
// 			}
// 		}
// 		return nil
// 	}); err != nil {
// 		log.Panic(err)
// 		return err
// 	}

// 	return nil
// }

func (r *RedisStore) GetSpend(ctx context.Context, outpoint string, refresh bool) (spend string, err error) {
	if spend, err = r.DB.HGet(ctx, SpendsKey, outpoint).Result(); err != nil && err != redis.Nil {
		return "", err
	} else if spend == "" && refresh {
		if spend, err = jb.GetSpend(outpoint); err != nil {
			return
		} else if spend != "" {
			if _, err = r.SetNewSpend(ctx, outpoint, spend); err != nil {
				return
			}
		}
	}
	return spend, nil
}

func (r *RedisStore) GetSpends(ctx context.Context, outpoints []string, refresh bool) ([]string, error) {
	spends := make([]string, 0, len(outpoints))
	if records, err := r.DB.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		for i, record := range records {
			outpoint := outpoints[i]
			var spend string
			if record != nil {
				spend = record.(string)
			} else if refresh {
				if spend, err = jb.GetSpend(outpoint); err != nil {
					return nil, err
				} else if spend != "" {
					if _, err = r.SetNewSpend(ctx, outpoint, spend); err != nil {
						return nil, err
					}
				}
			}
			spends = append(spends, spend)
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

func (r *RedisStore) Rollback(ctx context.Context, txid string) error {
	if outpoints, err := r.DB.SMembers(ctx, InputsKey(txid)).Result(); err != nil {
		log.Panic(err)
		return err
	} else if txos, err := r.LoadTxosByTxid(ctx, txid, nil, false, false); err != nil {
		log.Panic(err)
		return err
	} else {
		deletes := make([]string, 0, len(outpoints))
		if spends, err := r.GetSpends(ctx, outpoints, false); err != nil {
			return err
		} else {
			for i, spend := range spends {
				if spend == txid {
					deletes = append(deletes, outpoints[i])
				}
			}
		}
		if len(deletes) > 0 {
			if err := r.DB.HDel(ctx, SpendsKey, deletes...).Err(); err != nil {
				log.Panic(err)
				return err
			}
		}
		for _, txo := range txos {
			if err = r.RollbackTxo(ctx, txo); err != nil {
				log.Panic(err)
				return err
			}
		}
	}
	return nil
}

func (r *RedisStore) RollbackTxo(ctx context.Context, txo *idx.Txo) error {
	outpoint := txo.Outpoint.String()
	if _, err := r.DB.Pipelined(ctx, func(pipe redis.Pipeliner) (err error) {
		for _, owner := range txo.Owners {
			if owner == "" {
				continue
			}
			if err := pipe.ZRem(ctx, idx.OwnerKey(owner), outpoint).Err(); err != nil {
				log.Println("ZRem Owner", err)
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
