package idx

import (
	"context"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/jb"
)

const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:acct"

// var OwnerTag = "own"
// var AccountTag = "acct"

func AccountKey(account string) string {
	return "acct:" + account
}

func OwnerKey(owner string) string {
	return "own:" + owner
}

func OwnerTxosKey(owner string) string {
	return "own:" + owner + ":txo"
}

func AccountTxosKey(account string) string {
	return "acct:" + account + ":txo"
}

func AcctsByOwners(ctx context.Context, owners []string) ([]string, error) {
	if len(owners) == 0 {
		return nil, nil
	}
	acctSet := make(map[string]struct{}, len(owners))
	accts := make([]string, 0, len(owners))
	if ownAccts, err := AcctDB.HMGet(ctx, OwnerAccountKey, owners...).Result(); err != nil {
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

func SyncAcct(ctx context.Context, tag string, acct string, ing *IngestCtx) error {
	if owners, err := AcctDB.SMembers(ctx, AccountKey(acct)).Result(); err != nil {
		return err
	} else {
		for _, owner := range owners {
			if err := SyncOwner(ctx, tag, owner, ing); err != nil {
				return err
			}
		}
	}
	return nil
}

func SyncOwner(ctx context.Context, tag string, add string, ing *IngestCtx) error {
	log.Println("Syncing:", add)
	if lastHeight, err := AcctDB.ZScore(ctx, OwnerSyncKey, add).Result(); err != nil && err != redis.Nil {
		return err
	} else if addTxns, err := jb.FetchOwnerTxns(add, int(lastHeight)); err != nil {
		log.Panic(err)
	} else {
		limiter := make(chan struct{}, ing.Concurrency)
		var wg sync.WaitGroup
		for _, addTxn := range addTxns {
			if err := AcctDB.ZScore(ctx, LogKey(tag), addTxn.Txid).Err(); err != nil && err != redis.Nil {
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
				if _, err := ing.IngestTxid(ctx, addTxn.Txid, AncestorConfig{
					Load:  true,
					Parse: true,
				}); err != nil {
					log.Panic(err)
				}
			}(addTxn)

			if addTxn.Height > uint32(lastHeight) {
				lastHeight = float64(addTxn.Height)
			}
		}
		wg.Wait()
		if err := AcctDB.ZAdd(ctx, OwnerSyncKey, redis.Z{
			Score:  float64(lastHeight),
			Member: add,
		}).Err(); err != nil {
			log.Panic(err)
		}
	}

	return nil
}

func UpdateAccount(ctx context.Context, account string, owners []string) (bool, error) {
	accountKey := AccountKey(account)
	for _, owner := range owners {
		if owner == "" {
			continue
		}
		if added, err := AcctDB.ZAddNX(ctx, OwnerSyncKey, redis.Z{
			Score:  0,
			Member: owner,
		}).Result(); err != nil {
			return false, err
		} else if added == 0 {
			continue
		} else if err := AcctDB.HSet(ctx, OwnerAccountKey, owner, account).Err(); err != nil {
			return false, err
		} else if AcctDB.SAdd(ctx, accountKey, owner).Err() != nil {
			return false, err
		} else if err = TxoDB.ZRangeStore(ctx, AccountTxosKey(account), redis.ZRangeArgs{
			Key:   OwnerTxosKey(owner),
			Start: 0,
			Stop:  -1,
		}).Err(); err != nil {
			return false, err
		}
	}

	return true, nil
}
