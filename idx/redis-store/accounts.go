package redisstore

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/idx"
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
		for _, owner := range owners {
			if owner == "" {
				continue
			}
			ownerTxoKeys = append(ownerTxoKeys, idx.OwnerKey(owner))
			if _, exists := current[owner]; exists {
				log.Println("Owner already exists:", owner)
				// continue
			}
			if err := pipe.ZAddNX(ctx, idx.OwnerSyncKey, redis.Z{
				Score:  0,
				Member: owner,
			}).Err(); err != nil {
				return err
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

// func (r *RedisStore) SyncAcct(ctx context.Context, tag string, acct string, ing *idx.IngestCtx) error {
// 	if owners, err := r.AcctOwners(ctx, acct); err != nil {
// 		return err
// 	} else {
// 		for _, owner := range owners {
// 			if err := r.SyncOwner(ctx, tag, owner, ing); err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }

// func (r *RedisStore) SyncOwner(ctx context.Context, tag string, own string, ing *idx.IngestCtx) error {
// 	log.Println("Syncing:", own)
// 	if lastHeight, err := r.LogScore(ctx, idx.OwnerSyncKey, own); err != nil && err != redis.Nil {
// 		return err
// 	} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
// 		log.Panic(err)
// 	} else {
// 		limiter := make(chan struct{}, ing.Concurrency)
// 		var wg sync.WaitGroup
// 		for _, addTxn := range addTxns {
// 			if score, err := r.LogScore(ctx, tag, addTxn.Txid); err != nil {
// 				log.Panic(err)
// 				return err
// 			} else if score > 0 {
// 				continue
// 			}
// 			wg.Add(1)
// 			limiter <- struct{}{}
// 			go func(addTxn *jb.AddressTxn) {
// 				defer func() {
// 					<-limiter
// 					wg.Done()
// 				}()
// 				if _, err := ing.IngestTxid(ctx, addTxn.Txid, idx.AncestorConfig{
// 					Load:  true,
// 					Parse: true,
// 					Save:  true,
// 				}); err != nil {
// 					log.Panic(err)
// 				}
// 			}(addTxn)

// 			if addTxn.Height > uint32(lastHeight) {
// 				lastHeight = float64(addTxn.Height)
// 			}
// 		}
// 		wg.Wait()
// 		if err := r.Log(ctx, idx.OwnerSyncKey, own, lastHeight); err != nil {
// 			log.Panic(err)
// 		}
// 	}

// 	return nil
// }
