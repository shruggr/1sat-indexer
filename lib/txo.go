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
	Satoshis uint64                `json:"satoshis"`
	OutAcc   uint64                `json:"outacc"`
	Owners   []string              `json:"owners,omitempty"`
	Data     map[string]*IndexData `json:"-"`
}

func (t *Txo) AddOwner(owner string) {
	for _, o := range t.Owners {
		if o == owner {
			return
		}
	}
	t.Owners = append(t.Owners, owner)
}

func (txo *Txo) Save(ctx context.Context, idxCtx *IndexContext) error {
	outpoint := txo.Outpoint.String()
	score := HeightScore(idxCtx.Height, idxCtx.Idx, false)

	accts := map[string]struct{}{}
	if len(txo.Owners) > 0 {
		if result, err := Rdb.HMGet(ctx, OwnerAccountKey, txo.Owners...).Result(); err != nil {
			log.Panic(err)
			return err
		} else {
			for _, item := range result {
				if acct, ok := item.(string); ok && acct != "" {
					accts[acct] = struct{}{}
				}
			}
		}
	}
	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if mp, err := msgpack.Marshal(txo); err != nil {
			log.Panic(err)
			return err
		} else if err := pipe.HSet(ctx, TxosKey, outpoint, mp).Err(); err != nil {
			log.Println("HSET Txo", err)
			return err
		}
		for _, owner := range txo.Owners {
			if err := pipe.ZAdd(ctx, OwnerTxosKey(owner), redis.Z{
				Score:  score,
				Member: outpoint,
			}).Err(); err != nil {
				log.Println("ZADD Owner", err)
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
				return err
			}
		}
		for tag, data := range txo.Data {
			if data.Validate {
				if err := pipe.ZAdd(ctx, ValidateKey(tag), redis.Z{
					Score:  score,
					Member: outpoint,
				}).Err(); err != nil {
					log.Println("ZADD Validate", err)
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
		Rdb.Publish(ctx, PubOwnerKey(owner), outpoint)
	}
	for acct := range accts {
		Rdb.Publish(ctx, PubAccountKey(acct), outpoint)
	}
	for tag, data := range txo.Data {
		for _, event := range data.Events {
			Rdb.Publish(ctx, PubEventKey(tag, event), outpoint)
		}
	}
	return nil
}

func (spend *Txo) SaveSpend(ctx context.Context, idxCtx *IndexContext) error {
	score := HeightScore(idxCtx.Height, idxCtx.Idx, false)
	// outpoint := spend.Outpoint.String()
	txid := idxCtx.Txid.String()
	accts := map[string]struct{}{}
	if len(spend.Owners) > 0 {
		if result, err := Rdb.HMGet(ctx, OwnerAccountKey, spend.Owners...).Result(); err != nil {
			log.Panic(err)
			return err
		} else {
			for _, item := range result {
				if acct, ok := item.(string); ok && acct != "" {
					accts[acct] = struct{}{}
				}
			}
		}
	}
	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.HSet(ctx,
			SpendsKey,
			spend.Outpoint.String(),
			idxCtx.Txid.String(),
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
		Rdb.Publish(ctx, PubOwnerKey(owner), txid)
	}
	for acct := range accts {
		Rdb.Publish(ctx, PubAccountKey(acct), txid)
	}

	return nil
}

func (t Txo) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	m["outpoint"] = t.Outpoint
	m["height"] = t.Height
	m["idx"] = t.Idx
	m["satoshis"] = t.Satoshis
	m["outacc"] = t.OutAcc
	m["owners"] = t.Owners

	if len(t.Data) > 0 {
		d := map[string]any{}
		for tag, v := range t.Data {
			d[tag] = v.Data
		}
		m["data"] = d
	}
	return json.Marshal(m)
}
