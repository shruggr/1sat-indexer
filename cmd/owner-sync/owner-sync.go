package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var JUNGLEBUS string
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	if err := lib.Initialize(); err != nil {
		log.Panic(err)
	}
}

func main() {
	// indexers := make([]lib.Indexer, 0)
	for {
		iter := lib.Rdb.ZScan(ctx, lib.OwnerSyncKey, 0, "", 100).Iterator()
		for iter.Next(ctx) {
			add := iter.Val()
			iter.Next(ctx)
			log.Println("Address:", add)
			if lastHeight, err := strconv.Atoi(iter.Val()); err != nil {
				log.Panic(err)
			} else if addTxns, err := lib.FetchOwnerTxns(add, lastHeight); err != nil {
				log.Panic(err)
			} else {
				for _, addTxn := range addTxns {
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
				if err := lib.Rdb.ZAdd(ctx, lib.OwnerSyncKey, redis.Z{
					Score:  float64(lastHeight),
					Member: add,
				}).Err(); err != nil {
					log.Panic(err)
				}
				log.Println("Queued", add, lastHeight)
			}
		}
		time.Sleep(time.Minute)
	}
}
