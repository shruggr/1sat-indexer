package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/lib"
)

const PAGE_SIZE = 1000

var JUNGLEBUS string
var ctx = context.Background()

var CONCURRENCY int
var TAG string
var limiter chan struct{}

// var inflight map[string]struct{}

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

	if err := lib.Initialize(); err != nil {
		log.Panic(err)
	}
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
	limiter = make(chan struct{}, CONCURRENCY)
}

// var wg sync.WaitGroup
// var done = make(chan *lib.IndexContext, 1000)

// var queue = make(chan string, 1000)

// type TxQueueItem struct {
// 	txid     string
// 	score    float64
// 	parents  map[string]*TxQueueItem
// 	children map[string]*TxQueueItem
// }

// var txQueue = make(map[string]*TxQueueItem, 1000)

func main() {
	log.Println("CONCURRENCY:", CONCURRENCY)

	for {
		if members, err := lib.Queue.ZRangeWithScores(ctx, lib.PendingQueueKey, 0, PAGE_SIZE).Result(); err != nil {
			log.Panic(err)
		} else if len(members) == 0 {
			break
		} else {
			for _, member := range members {
				if err := lib.Queue.ZAdd(ctx, lib.IngestQueueKey, member).Err(); err != nil {
					log.Panic(err)
				} else if err := lib.Queue.ZRem(ctx, lib.PendingQueueKey, member.Member).Err(); err != nil {
					log.Panic(err)
				}
			}
		}
	}

	txcount := 0
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		for range ticker.C {
			log.Println("Transactions - Ingested", txcount, txcount/15, "tx/s")
			txcount = 0
		}
	}()
	for {
		// log.Println("Loading", PAGE_SIZE, "transactions to ingest")
		if txids, err := lib.Queue.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key:   lib.IngestQueueKey,
			Start: 0,
			Stop:  PAGE_SIZE - 1,
		}).Result(); err != nil {
			log.Panic(err)
		} else {
			for _, txid := range txids {
				limiter <- struct{}{}
				// wg.Add(1)
				go func(txid string) {
					defer func() {
						txcount++
						<-limiter
						// log.Println("Processed", txid)
					}()
					if _, err := lib.IngestTxid(ctx, txid, config.Indexers); err != nil {
						log.Panic(err)
					}
				}(txid)
			}
			// wg.Wait()
			if len(txids) == 0 {
				log.Println("No transactions to ingest")
				time.Sleep(time.Second)
			}
			// panic("done")
		}
	}
}
