package lib

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

type IndexDataMap map[string]*IndexData

type Txo struct {
	Outpoint *Outpoint             `json:"outpoint"`
	Height   uint32                `json:"height"`
	Idx      uint64                `json:"idx"`
	Satoshis *uint64               `json:"satoshis"`
	OutAcc   uint64                `json:"-"`
	Owners   []string              `json:"owners,omitempty"`
	Data     map[string]*IndexData `json:"data,omitempty" msgpack:"-"`
}

func (t *Txo) AddOwner(owner string) {
	for _, o := range t.Owners {
		if o == owner {
			return
		}
	}
	t.Owners = append(t.Owners, owner)
}

func LoadTxo(ctx context.Context, outpoint string, tags []string) (*Txo, error) {
	if result, err := Rdb.HGet(ctx, TxosKey, outpoint).Bytes(); err == redis.Nil {
		return nil, nil
	} else if err != nil {
		log.Panic(err)
		return nil, err
	} else {
		txo := &Txo{
			Data: make(map[string]*IndexData),
		}
		if err := msgpack.Unmarshal(result, txo); err != nil {
			log.Panic(err)
			return nil, err
		}
		if len(tags) > 0 {
			if datas, err := Rdb.HMGet(ctx, TxoDataKey(outpoint), tags...).Result(); err != nil {
				log.Panic(err)
				return nil, err
			} else {
				for i, tag := range tags {
					data := datas[i]
					if data != nil {
						txo.Data[tag] = &IndexData{
							Data: json.RawMessage(data.(string)),
						}
					}
				}
			}
		}
		return txo, nil
	}
}

func LoadTxos(ctx context.Context, outpoints []string, tags []string) ([]*Txo, error) {
	if msgpacks, err := Rdb.HMGet(ctx, TxosKey, outpoints...).Result(); err != nil {
		return nil, err
	} else {
		txos := make([]*Txo, 0, len(msgpacks))
		for i, mp := range msgpacks {
			var txo *Txo
			if mp != nil {
				outpoint := outpoints[i]
				txo = &Txo{
					Data: make(map[string]*IndexData),
				}
				if err = msgpack.Unmarshal([]byte(mp.(string)), txo); err != nil {
					return nil, err
				}
				if len(tags) > 0 {
					if datas, err := Rdb.HMGet(ctx, TxoDataKey(outpoint), tags...).Result(); err != nil {
						log.Panic(err)
						return nil, err
					} else {
						for i, tag := range tags {
							data := datas[i]
							if data != nil {
								txo.Data[tag] = &IndexData{
									Data: json.RawMessage(data.(string)),
								}
							}
						}
					}
				}
			}
			txos = append(txos, txo)
		}
		return txos, nil
	}
}

func (txo *Txo) Save(ctx context.Context, height uint32, idx uint64) error {
	outpoint := txo.Outpoint.String()
	score := HeightScore(height, idx)

	accts := map[string]struct{}{}
	for _, owner := range txo.Owners {
		if acct, ok := OwnerAccounts[owner]; ok {
			accts[acct] = struct{}{}
		}
	}

	if mp, err := msgpack.Marshal(txo); err != nil {
		log.Println("Marshal Txo", err)
		log.Panic(err)
		return err
	} else if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) (err error) {
		if err := pipe.HSet(ctx, TxosKey, outpoint, mp).Err(); err != nil {
			log.Println("HSET Txo", err)
			log.Panic(err)
			return err
		}
		for _, owner := range txo.Owners {
			if err := pipe.ZAdd(ctx, OwnerTxosKey(owner), redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Owner", err)
				log.Panic(err)
				return err
			}
		}
		for acct := range accts {
			// logs = append(logs, fmt.Sprintf("%d %s %015f", idxCtx.Id, outpoint, score))
			if err := pipe.ZAdd(ctx, AccountTxosKey(acct), redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Account", err)
				log.Panic(err)
				return err
			}
		}
		if len(txo.Data) > 0 {
			datas := make(map[string]any, len(txo.Data))
			for tag, data := range txo.Data {
				// if len(data.DepQueue) > 0 {
				// 	txo.DepQueue = append(txo.DepQueue, data.DepQueue...)
				// 	continue
				// }
				tagKey := TagKey(tag)
				if err := pipe.ZAdd(ctx, tagKey, redis.Z{
					Score:  score,
					Member: outpoint,
				}).Err(); err != nil {
					log.Println("ZADD Tag", tagKey, err)
					log.Panic(err)
					return err
				}
				for _, event := range data.Events {
					eventKey := EventKey(tag, event)
					if err := pipe.ZAdd(ctx, eventKey, redis.Z{
						Score:  score,
						Member: outpoint,
					}).Err(); err != nil {
						log.Panic(err)
						log.Println("ZADD Event", eventKey, err)
						return err
					}
				}
				// if datas[tag], err = msgpack.Marshal(data); err != nil {
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
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}
	for _, owner := range txo.Owners {
		Queue.Publish(ctx, PubOwnerKey(owner), outpoint)
	}
	for acct := range accts {
		Queue.Publish(ctx, PubAccountKey(acct), outpoint)
	}
	for tag, data := range txo.Data {
		for _, event := range data.Events {
			Queue.Publish(ctx, PubEventKey(tag, event), outpoint)
		}
	}
	return nil
}

func (spend *Txo) SaveSpend(ctx context.Context, txid string, height uint32, idx uint64) error {
	score := HeightScore(height, idx)
	accts := map[string]struct{}{}
	for _, owner := range spend.Owners {
		if acct, ok := OwnerAccounts[owner]; ok {
			accts[acct] = struct{}{}
		}
	}
	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.HSet(ctx,
			SpendsKey,
			spend.Outpoint.String(),
			txid,
		).Err(); err != nil {
			return err
		}
		for _, owner := range spend.Owners {
			ownerKey := OwnerTxosKey(owner)
			if err := pipe.ZAdd(ctx, ownerKey, redis.Z{
				Score:  score,
				Member: txid,
			}).Err(); err != nil {
				return err
			}
		}
		for acct := range accts {
			if err := pipe.ZAdd(ctx, AccountTxosKey(acct), redis.Z{
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

	for _, owner := range spend.Owners {
		Queue.Publish(ctx, PubOwnerKey(owner), txid)
	}
	for acct := range accts {
		Queue.Publish(ctx, PubAccountKey(acct), txid)
	}

	return nil
}

func (t *Txo) SaveData(ctx context.Context, tags []string) (err error) {
	if len(tags) == 0 {
		return nil
	}

	outpoint := t.Outpoint.String()
	score := HeightScore(t.Height, t.Idx)
	datas := make(map[string]any, len(tags))
	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, tag := range tags {
			if data, ok := t.Data[tag]; ok {
				tagKey := TagKey(tag)
				if err := pipe.ZAdd(ctx, tagKey, redis.Z{
					Score:  score,
					Member: outpoint,
				}).Err(); err != nil {
					log.Println("ZADD Tag", tagKey, err)
					return err
				}
				for _, event := range data.Events {
					eventKey := EventKey(tag, event)
					if err := pipe.ZAdd(ctx, eventKey, redis.Z{
						Score:  score,
						Member: outpoint,
					}).Err(); err != nil {
						return err
					}
				}
				// if datas[tag], err = msgpack.Marshal(data); err != nil {
				if datas[tag], err = data.MarshalJSON(); err != nil {
					log.Panic(err)
					return err
				}
			}
		}
		if err := pipe.HMSet(ctx, TxoDataKey(outpoint), datas).Err(); err != nil {
			log.Println("HMSET TxoData", err)
			return err
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}
	for _, tag := range tags {
		for _, event := range t.Data[tag].Events {
			Queue.Publish(ctx, PubEventKey(tag, event), outpoint)
		}
	}
	return nil
}
