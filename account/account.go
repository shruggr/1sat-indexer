package acct

import (
	"context"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/cmd/ingest/ingest"
	"github.com/shruggr/1sat-indexer/lib"
)

func SyncAcct(ctx context.Context, acct string, ing *ingest.Ingest) error {
	if owners, err := lib.Queue.SMembers(ctx, lib.AccountKey(acct)).Result(); err != nil {
		return err
	} else {
		for _, owner := range owners {
			if err := SyncOwner(ctx, owner, ing); err != nil {
				return err
			}
		}
	}
	return nil
}

func SyncOwner(ctx context.Context, add string, ing *ingest.Ingest) error {
	log.Println("Syncing:", add)
	if lastHeight, err := lib.Queue.ZScore(ctx, lib.OwnerSyncKey, add).Result(); err != nil && err != redis.Nil {
		return err
	} else if addTxns, err := lib.FetchOwnerTxns(add, int(lastHeight)); err != nil {
		log.Panic(err)
	} else {
		limiter := make(chan struct{}, ing.Concurrency)
		var wg sync.WaitGroup
		for _, addTxn := range addTxns {
			if err := lib.Queue.ZScore(ctx, lib.IngestLogKey, addTxn.Txid).Err(); err != nil && err != redis.Nil {
				return err
			} else if err != redis.Nil {
				continue
			}
			wg.Add(1)
			limiter <- struct{}{}
			go func(addTxn *lib.AddressTxn) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if _, err := ing.IngestTxid(ctx, addTxn.Txid); err != nil {
					log.Panic(err)
				}
				log.Println("Ingested", addTxn.Txid)
			}(addTxn)

			if addTxn.Height > uint32(lastHeight) {
				lastHeight = float64(addTxn.Height)
			}
		}
		wg.Wait()
		if err := lib.Queue.ZAdd(ctx, lib.OwnerSyncKey, redis.Z{
			Score:  float64(lastHeight),
			Member: add,
		}).Err(); err != nil {
			log.Panic(err)
		}
	}

	return nil
}

func AccountUtxos(ctx context.Context, acct string, tags []string) ([]*lib.Txo, error) {
	if scores, err := lib.Rdb.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key:   lib.AccountTxosKey(acct),
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
		if spends, err := lib.Rdb.HMGet(ctx, lib.SpendsKey, outpoints...).Result(); err != nil {
			return nil, err
		} else {
			unspent := make([]string, 0, len(outpoints))
			for i, outpoint := range outpoints {
				if spends[i] == nil {
					unspent = append(unspent, outpoint)
				}
			}

			if txos, err := lib.LoadTxos(ctx, unspent, tags); err != nil {
				return nil, err
			} else {
				return txos, err
			}
		}
	}
}

func AddressUtxos(ctx context.Context, address string, tags []string) ([]*lib.Txo, error) {
	if scores, err := lib.Rdb.ZRangeArgsWithScores(ctx, redis.ZRangeArgs{
		Key:   lib.OwnerTxosKey(address),
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
		if spends, err := lib.Rdb.HMGet(ctx, lib.SpendsKey, outpoints...).Result(); err != nil {
			return nil, err
		} else {
			unspent := make([]string, 0, len(outpoints))
			for i, outpoint := range outpoints {
				if spends[i] == nil {
					unspent = append(unspent, outpoint)
				}
			}

			if txos, err := lib.LoadTxos(ctx, unspent, tags); err != nil {
				return nil, err
			} else {
				return txos, err
			}
		}
	}
}
