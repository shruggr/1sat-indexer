package lib

import (
	"context"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

func SyncAcct(ctx context.Context, acct string, indexers []Indexer, concurrency uint8) error {
	if owners, err := Queue.SMembers(ctx, AccountKey(acct)).Result(); err != nil {
		return err
	} else {
		for _, owner := range owners {
			if err := SyncOwner(ctx, owner, indexers, concurrency); err != nil {
				return err
			}
		}
	}
	return nil
}

func SyncOwner(ctx context.Context, add string, indexers []Indexer, concurrency uint8) error {
	log.Println("Syncing:", add)
	if lastHeight, err := Queue.ZScore(ctx, OwnerSyncKey, add).Result(); err != nil && err != redis.Nil {
		return err
	} else if addTxns, err := FetchOwnerTxns(add, int(lastHeight)); err != nil {
		log.Panic(err)
	} else {
		limiter := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		for _, addTxn := range addTxns {
			// if err := Queue.ZScore(ctx, IngestLogKey, addTxn.Txid).Err(); err != nil && err != redis.Nil {
			// 	return err
			// } else if err != redis.Nil {
			// 	continue
			// }
			wg.Add(1)
			limiter <- struct{}{}
			go func(addTxn *AddressTxn) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if _, err := IngestTxid(ctx, addTxn.Txid, indexers); err != nil {
					log.Panic(err)
				}
				log.Println("Ingested", addTxn.Txid)
			}(addTxn)

			if addTxn.Height > uint32(lastHeight) {
				lastHeight = float64(addTxn.Height)
			}
		}
		wg.Wait()
		if err := Queue.ZAdd(ctx, OwnerSyncKey, redis.Z{
			Score:  float64(lastHeight),
			Member: add,
		}).Err(); err != nil {
			log.Panic(err)
		}
	}

	return nil
}

func AccountUtxos(ctx context.Context, acct string, tags []string) ([]*Txo, error) {
	if scores, err := Rdb.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key:   AccountTxosKey(acct),
		Start: 0,
		Stop:  -1,
	}).Result(); err != nil {
		return nil, err
	} else {
		outpoints := make([]string, 0, len(scores))
		for _, item := range scores {
			member := item.Member.(string)
			if len(member) > 64 {
				outpoints = append(outpoints, member)
			}
		}
		if spends, err := Rdb.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
			return nil, err
		} else {
			unspent := make([]string, 0, len(outpoints))
			for i, outpoint := range outpoints {
				if spends[i] == nil {
					unspent = append(unspent, outpoint)
				}
			}

			if txos, err := LoadTxos(ctx, unspent, tags); err != nil {
				return nil, err
			} else {
				return txos, err
			}
		}
	}
}

func AddressUtxos(ctx context.Context, address string, tags []string) ([]*Txo, error) {
	if scores, err := Rdb.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key:   OwnerTxosKey(address),
		Start: 0,
		Stop:  -1,
	}).Result(); err != nil {
		return nil, err
	} else {
		outpoints := make([]string, 0, len(scores))
		for _, item := range scores {
			member := item.Member.(string)
			if len(member) > 64 {
				outpoints = append(outpoints, member)
			}
		}
		if spends, err := Rdb.HMGet(ctx, SpendsKey, outpoints...).Result(); err != nil {
			return nil, err
		} else {
			unspent := make([]string, 0, len(outpoints))
			for i, outpoint := range outpoints {
				if spends[i] == nil {
					unspent = append(unspent, outpoint)
				}
			}

			if txos, err := LoadTxos(ctx, unspent, tags); err != nil {
				return nil, err
			} else {
				return txos, err
			}
		}
	}
}
