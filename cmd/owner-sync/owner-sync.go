package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var ctx = context.Background()
var CONCURRENCY int

func init() {
	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

}

// func main() {
// 	for {
// 		iter := lib.Queue.ZScan(ctx, lib.OwnerSyncKey, 0, "", 100).Iterator()
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
		if err := lib.RefreshOwners(); err != nil {
			log.Panic(err)
		}
		iter := lib.Queue.ZScan(ctx, lib.OwnerSyncKey, 0, "", 100).Iterator()
		for iter.Next(ctx) {
			add := iter.Val()
			iter.Next(ctx)

			if lastHeight, err := strconv.Atoi(iter.Val()); err != nil {
				log.Panic(err)
			} else if addTxns, err := lib.FetchOwnerTxns(add, lastHeight); err != nil {
				log.Panic(err)
			} else {
				for _, addTxn := range addTxns {
					if err := lib.Queue.ZScore(ctx, lib.IngestLogKey, addTxn.Txid).Err(); err != nil && err != redis.Nil {
						log.Panic(err)
					} else if err != redis.Nil {
						continue
					}
					score, _ := strconv.ParseFloat(fmt.Sprintf("%07d.%09d", addTxn.Height, addTxn.Idx), 64)
					if err := lib.Queue.ZAdd(ctx, lib.IngestQueueKey, redis.Z{
						Score:  score,
						Member: addTxn.Txid,
					}).Err(); err != nil {
						log.Panic(err)
					}
					log.Println("Queuing", addTxn.Txid, score)

					if addTxn.Height > uint32(lastHeight) {
						lastHeight = int(addTxn.Height)
					}
				}
				if err := lib.Queue.ZAdd(ctx, lib.OwnerSyncKey, redis.Z{
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
