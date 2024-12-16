package redisstore

import (
	"context"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

func (r *RedisStore) AcctsByOwners(ctx context.Context, owners []string) ([]string, error) {
	if len(owners) == 0 {
		return nil, nil
	}
	acctSet := make(map[string]struct{}, len(owners))
	accts := make([]string, 0, len(owners))
	if ownAccts, err := r.DB.HMGet(ctx, idx.OwnerAccountKey, owners...).Result(); err != nil {
		return nil, err
	} else {
		for _, acct := range ownAccts {
			if a, ok := acct.(string); ok {
				if _, exists := acctSet[a]; !exists {
					acctSet[a] = struct{}{}
					accts = append(accts, a)
				}
			}
		}
	}
	return accts, nil
}

func (r *RedisStore) AcctOwners(ctx context.Context, account string) ([]string, error) {
	return r.DB.SMembers(ctx, idx.AccountKey(account)).Result()
}

func (r *RedisStore) UpdateAccount(ctx context.Context, account string, owners []string) error {
	accountKey := idx.AccountKey(account)
	accountTxosKey := idx.AccountTxosKey(account)
	ownerTxoKeys := make([]string, 0, len(owners))
	current := make(map[string]struct{})
	if currOwners, err := r.DB.SMembers(ctx, idx.AccountKey(account)).Result(); err != nil {
		return err
	} else {
		for _, owner := range currOwners {
			current[owner] = struct{}{}
		}
	}
	_, err := r.DB.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		unionOwners := false
		for _, owner := range owners {
			if owner == "" {
				continue
			}
			ownerTxoKeys = append(ownerTxoKeys, idx.OwnerTxosKey(owner))
			if _, exists := current[owner]; exists {
				log.Println("Owner already exists:", owner)
				// continue
			}
			unionOwners = true
			if err := pipe.ZAddNX(ctx, idx.OwnerSyncKey, redis.Z{
				Score:  0,
				Member: owner,
			}).Err(); err != nil {
				return err
			}
		}

		if unionOwners {
			if txoCount, err := pipe.ZUnionStore(ctx, accountTxosKey, &redis.ZStore{
				Keys:      ownerTxoKeys,
				Aggregate: "MIN",
			}).Result(); err != nil {
				return err
			} else {
				log.Println("Added", txoCount, "txos to", accountTxosKey)
			}
		}

		for _, owner := range owners {
			if owner == "" {
				continue
			}

			if err := pipe.SAdd(ctx, accountKey, owner).Err(); err != nil {
				return err
			} else if err := pipe.HSet(ctx, idx.OwnerAccountKey, owner, account).Err(); err != nil {
				return err
			}
		}
		return nil
	})

	return err
}

func (r *RedisStore) SyncAcct(ctx context.Context, tag string, acct string, ing *idx.IngestCtx) error {
	if owners, err := r.DB.SMembers(ctx, idx.AccountKey(acct)).Result(); err != nil {
		return err
	} else {
		for _, owner := range owners {
			if err := r.SyncOwner(ctx, tag, owner, ing); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RedisStore) SyncOwner(ctx context.Context, tag string, own string, ing *idx.IngestCtx) error {
	log.Println("Syncing:", own)
	r.LogScore(ctx, tag, own)
	if lastHeight, err := r.DB.ZScore(ctx, idx.OwnerSyncKey, own).Result(); err != nil && err != redis.Nil {
		return err
	} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
		log.Panic(err)
	} else {
		limiter := make(chan struct{}, ing.Concurrency)
		var wg sync.WaitGroup
		for _, addTxn := range addTxns {
			if err := r.DB.ZScore(ctx, idx.LogKey(tag), addTxn.Txid).Err(); err != nil && err != redis.Nil {
				return err
			} else if err != redis.Nil {
				continue
			}
			wg.Add(1)
			limiter <- struct{}{}
			go func(addTxn *jb.AddressTxn) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				// score := idx.HeightScore(addTxn.Height, addTxn.Idx)
				// if err := r.Log(ctx, idx.IngestQueueKey, addTxn.Txid, score); err != nil {
				// 	log.Panic(err)
				// }
				// log.Println("Queued:", addTxn.Txid)
				if _, err := ing.IngestTxid(ctx, addTxn.Txid, idx.AncestorConfig{
					Load:  true,
					Parse: true,
					Save:  true,
				}); err != nil {
					log.Panic(err)
				}
			}(addTxn)

			if addTxn.Height > uint32(lastHeight) {
				lastHeight = float64(addTxn.Height)
			}
		}
		wg.Wait()
		if err := r.DB.ZAdd(ctx, idx.OwnerSyncKey, redis.Z{
			Score:  float64(lastHeight),
			Member: own,
		}).Err(); err != nil {
			log.Panic(err)
		}
	}

	return nil
}
