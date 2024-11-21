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
	if ownAccts, err := r.TxoDB.HMGet(ctx, idx.OwnerAccountKey, owners...).Result(); err != nil {
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
	return r.TxoDB.SMembers(ctx, idx.AccountKey(account)).Result()
}

func (r *RedisStore) UpdateAccount(ctx context.Context, account string, owners []string) error {
	accountKey := idx.AccountKey(account)
	for _, owner := range owners {
		if owner == "" {
			continue
		}
		if added, err := r.TxoDB.ZAddNX(ctx, idx.OwnerSyncKey, redis.Z{
			Score:  0,
			Member: owner,
		}).Result(); err != nil {
			return err
		} else if added == 0 {
			continue
		} else if err := r.TxoDB.HSet(ctx, idx.OwnerAccountKey, owner, account).Err(); err != nil {
			return err
		} else if r.TxoDB.SAdd(ctx, accountKey, owner).Err() != nil {
			return err
		} else if err = r.TxoDB.ZRangeStore(ctx, idx.AccountTxosKey(account), redis.ZRangeArgs{
			Key:   idx.OwnerTxosKey(owner),
			Start: 0,
			Stop:  -1,
		}).Err(); err != nil {
			return err
		}
	}

	return nil
}

func (r *RedisStore) SyncAcct(ctx context.Context, tag string, acct string, ing *idx.IngestCtx) error {
	if owners, err := r.AcctDB.SMembers(ctx, idx.AccountKey(acct)).Result(); err != nil {
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
	if lastHeight, err := r.AcctDB.ZScore(ctx, idx.OwnerSyncKey, own).Result(); err != nil && err != redis.Nil {
		return err
	} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
		log.Panic(err)
	} else {
		limiter := make(chan struct{}, ing.Concurrency)
		var wg sync.WaitGroup
		for _, addTxn := range addTxns {
			if err := r.AcctDB.ZScore(ctx, idx.LogKey(tag), addTxn.Txid).Err(); err != nil && err != redis.Nil {
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
		if err := r.AcctDB.ZAdd(ctx, idx.OwnerSyncKey, redis.Z{
			Score:  float64(lastHeight),
			Member: own,
		}).Err(); err != nil {
			log.Panic(err)
		}
	}

	return nil
}
