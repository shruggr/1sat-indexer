package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

var ctx = context.Background()
var CONCURRENCY int
var TAG string
var store idx.TxoStore

func init() {
	flag.StringVar(&TAG, "tag", idx.IngestTag, "Ingest tag")
	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()
	store = config.Store
}

func main() {
	for {
		if results, err := store.Search(ctx, &idx.SearchCfg{
			Keys: []string{idx.OwnerSyncKey},
		}); err != nil {
			log.Panic(err)
		} else if len(results) == 0 {
			time.Sleep(time.Minute)
			continue
		} else {
			for _, result := range results {
				lastHeight := int(result.Score)
				log.Println("Owner", result.Member, "lastHeight", lastHeight)
				if addTxns, err := jb.FetchOwnerTxns(result.Member, lastHeight); err != nil {
					log.Panic(err)
				} else {
					for _, addTxn := range addTxns {
						if score, err := store.LogScore(ctx, idx.LogKey(TAG), addTxn.Txid); err != nil && err != redis.Nil {
							log.Panic(err)
						} else if score > 0 {
							continue
						}
						score := idx.HeightScore(addTxn.Height, addTxn.Idx)
						if err := store.Log(ctx, idx.QueueKey(TAG), addTxn.Txid, score); err != nil {
							log.Panic(err)
						}
						log.Println("Ingesting", addTxn.Txid, score)

						if addTxn.Height > uint32(lastHeight) {
							lastHeight = int(addTxn.Height)
						}
					}

					if err := store.Log(ctx, idx.OwnerSyncKey, result.Member, float64(lastHeight)); err != nil {
						log.Panic(err)
					}
				}
			}
		}
		time.Sleep(time.Minute)
	}
}
