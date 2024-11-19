package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/idx"
	redisstore "github.com/shruggr/1sat-indexer/v5/idx/redis-store"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

var ctx = context.Background()
var CONCURRENCY int
var TAG string
var store *redisstore.RedisStore

func init() {
	flag.StringVar(&TAG, "tag", "ingest", "Ingest tag")
	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()
	store = redisstore.NewRedisTxoStore(os.Getenv("REDISTXO"))
}

func main() {
	for {
		iter := idx.AcctDB.ZScan(ctx, redisstore.OwnerSyncKey, 0, "", 100).Iterator()
		for iter.Next(ctx) {
			add := iter.Val()
			iter.Next(ctx)

			if lastHeight, err := strconv.Atoi(iter.Val()); err != nil {
				log.Panic(err)
			} else if addTxns, err := jb.FetchOwnerTxns(add, lastHeight); err != nil {
				log.Panic(err)
			} else {
				for _, addTxn := range addTxns {
					if score, err := store.LogScore(ctx, TAG, addTxn.Txid); err != nil && err != redis.Nil {
						log.Panic(err)
					} else if score > 0 {
						continue
					}
					score := idx.HeightScore(addTxn.Height, addTxn.Idx)
					if err := store.Log(ctx, TAG, addTxn.Txid, score); err != nil {
						log.Panic(err)
					}
					log.Println("Ingesting", addTxn.Txid, score)

					if addTxn.Height > uint32(lastHeight) {
						lastHeight = int(addTxn.Height)
					}
				}
				if err := idx.AcctDB.ZAdd(ctx, redisstore.OwnerSyncKey, redis.Z{
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
