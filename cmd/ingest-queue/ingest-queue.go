package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/ingest"
	"github.com/shruggr/1sat-indexer/lib"
)

var JUNGLEBUS string
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

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
	limiter := make(chan struct{}, 4)
	var wg sync.WaitGroup
	indexers := []lib.Indexer{
		&bopen.BOpenIndexer{},
		&bopen.InscriptionIndexer{},
		&bopen.MapIndexer{},
		&bopen.BIndexer{},
		&bopen.SigmaIndexer{},
		&bopen.Bsv21Indexer{},
		&bopen.Bsv20Indexer{},
	}
	for {
		if txids, err := lib.Rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key:   "ingest",
			Start: 0,
			Stop:  100,
		}).Result(); err != nil {
			log.Panic(err)
		} else {
			for _, txid := range txids {
				limiter <- struct{}{}
				wg.Add(1)
				go func(txid string) {
					defer func() {
						<-limiter
						wg.Done()
					}()
					log.Println("Processing", txid)
					if status, err := lib.Rdb.ZScore(ctx, "status", txid).Result(); err != nil && err != redis.Nil {
						log.Panic(err)
					} else if status < 1 {
						if tx, err := lib.LoadTx(ctx, txid); err != nil {
							log.Panic(err)
						} else if _, err = ingest.IngestTx(ctx, tx, indexers); err != nil {
							log.Panic(err)
						}
					} else {
						log.Println("Skipping", status, txid)
					}
					if err := lib.Rdb.ZRem(ctx, "ingest", txid).Err(); err != nil {
						log.Panic(err)
					} else {
						log.Println("Ingested", txid)
					}
				}(txid)
			}
			if len(txids) == 0 {
				log.Println("No transactions to ingest")
				time.Sleep(time.Second)
			}
			wg.Wait()
		}
	}
}
