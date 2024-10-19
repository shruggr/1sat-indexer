package main

import (
	"context"
	"flag"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/jb"
)

var ctx = context.Background()
var CONCURRENCY int

func init() {
	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

}

// func main() {
// 	for {
// 		iter := data.Queue.ZScan(ctx, lib.OwnerSyncKey, 0, "", 100).Iterator()
// 		for iter.Next(ctx) {
// 			add := iter.Val()
// 			iter.Next(ctx)

// 			acct.SyncOwner(ctx, add, config.Indexers, uint8(CONCURRENCY))
// 		}
// 		time.Sleep(time.Minute)
// 	}
// }

func main() {
	for {
		iter := idx.AcctDB.ZScan(ctx, idx.OwnerSyncKey, 0, "", 100).Iterator()
		for iter.Next(ctx) {
			add := iter.Val()
			iter.Next(ctx)

			if lastHeight, err := strconv.Atoi(iter.Val()); err != nil {
				log.Panic(err)
			} else if addTxns, err := jb.FetchOwnerTxns(add, lastHeight); err != nil {
				log.Panic(err)
			} else {
				for _, addTxn := range addTxns {
					if exists, err := idx.TxIngested(ctx, addTxn.Txid); err != nil && err != redis.Nil {
						log.Panic(err)
					} else if exists {
						continue
					}
					score := idx.HeightScore(addTxn.Height, addTxn.Idx)
					if err := idx.QueneTx(ctx, addTxn.Txid, score); err != nil {
						log.Panic(err)
					}
					log.Println("Ingesting", addTxn.Txid, score)

					if addTxn.Height > uint32(lastHeight) {
						lastHeight = int(addTxn.Height)
					}
				}
				if err := idx.AcctDB.ZAdd(ctx, idx.OwnerSyncKey, redis.Z{
					Score:  float64(lastHeight),
					Member: add,
				}).Err(); err != nil {
					log.Panic(err)
				}
			}
		}
		time.Sleep(time.Minute)
	}
}
