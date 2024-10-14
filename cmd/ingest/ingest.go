package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/lib"
)

const CONCURRENCY = 32

var JUNGLEBUS string
var ctx = context.Background()

var TAG string

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.Parse()

	var err error
	db, err := pgxpool.New(ctx, os.Getenv("POSTGRES_FULL"))
	if err != nil {
		log.Panic(err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	cache := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISCACHE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	if err = lib.Initialize(db, rdb, cache); err != nil {
		log.Panic(err)
	}
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
}

func main() {
	limiter := make(chan struct{}, CONCURRENCY)
	var wg sync.WaitGroup
	indexers := []lib.Indexer{
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
	var m sync.Mutex
	inflight := make(map[string]struct{}, CONCURRENCY)
	for {
		ingetsKey := lib.IngestQueueKey(TAG)
		logKey := lib.IngestLogKey(TAG)
		log.Println("Loading transactions to ingest")
		if txids, err := lib.Rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key:   ingetsKey,
			Start: 0,
			Stop:  1000,
		}).Result(); err != nil {
			log.Panic(err)
		} else {
			for _, txid := range txids {
				m.Lock()
				if _, ok := inflight[txid]; ok {
					m.Unlock()
					continue
				}
				inflight[txid] = struct{}{}
				m.Unlock()
				limiter <- struct{}{}
				wg.Add(1)
				go func(txid string) {
					defer func() {
						m.Lock()
						delete(inflight, txid)
						m.Unlock()
						<-limiter
						wg.Done()
					}()
					// log.Println("Processing", txid)
					if tx, err := lib.LoadTx(ctx, txid); err != nil {
						log.Panic(err)
					} else if idxCtx, err := lib.IngestTx(ctx, tx, indexers); err != nil {
						log.Panic(err)
					} else if err := lib.Rdb.ZAdd(ctx, logKey, redis.Z{
						Score:  lib.HeightScore(idxCtx.Height, idxCtx.Idx),
						Member: txid,
					}).Err(); err != nil {
						log.Panic(err)
					} else if err := lib.Rdb.ZRem(ctx, ingetsKey, txid).Err(); err != nil {
						log.Panic(err)
					}
				}(txid)
			}
			if len(txids) == 0 {
				log.Println("No transactions to ingest")
				wg.Wait()
				time.Sleep(time.Second)
			}
		}
	}
}
