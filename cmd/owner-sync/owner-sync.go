package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/lib"
)

var ctx = context.Background()
var CONCURRENCY int

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

	if err := lib.Initialize(); err != nil {
		log.Panic(err)
	}
}

var indexers = []lib.Indexer{
	&bopen.BOpenIndexer{},
	&bopen.InscriptionIndexer{},
	&bopen.MapIndexer{},
	&bopen.BIndexer{},
	&bopen.SigmaIndexer{},
	&bopen.OriginIndexer{},
	&bopen.Bsv21Indexer{},
	&bopen.Bsv20Indexer{},
	&bopen.OrdLockIndexer{},
}

func main() {
	limiter := make(chan struct{}, CONCURRENCY)
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
				var wg sync.WaitGroup
				for _, addTxn := range addTxns {
					wg.Add(1)
					limiter <- struct{}{}
					go func(addTxn *lib.AddressTxn) {
						defer func() {
							<-limiter
							wg.Done()
						}()
						if _, err := lib.IngestTxid(ctx, addTxn.Txid, indexers); err != nil {
							log.Panic(err)
						}
						log.Println("Ingested", addTxn.Txid)
					}(addTxn)
					// score, _ := strconv.ParseFloat(fmt.Sprintf("%07d.%09d", addTxn.Height, addTxn.Idx), 64)
					// if err := lib.Queue.ZAdd(ctx, lib.IngestQueueKey, redis.Z{
					// 	Score:  score,
					// 	Member: addTxn.Txid,
					// }).Err(); err != nil {
					// 	log.Panic(err)
					// }
					// log.Println("Queuing", addTxn.Txid, score)

					if addTxn.Height > uint32(lastHeight) {
						lastHeight = int(addTxn.Height)
					}
				}
				wg.Wait()
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
