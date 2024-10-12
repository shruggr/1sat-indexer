package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/lib"
)

const CONCURRENCY = 10

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
	// indexers := []lib.Indexer{
	// 	&bopen.BOpenIndexer{},
	// 	&bopen.InscriptionIndexer{},
	// 	&bopen.MapIndexer{},
	// 	&bopen.SigmaIndexer{},
	// 	&bopen.OriginIndexer{},
	// }
	originIndexer := &bopen.OriginIndexer{}
	for {
		if outpoints, err := lib.Rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key:   lib.PostProcessingKey(originIndexer.Tag()),
			Start: 0,
			Stop:  1000,
		}).Result(); err != nil {
			log.Panic(err)
		} else {
			for _, op := range outpoints {
				if outpoint, err := lib.NewOutpointFromString(op); err != nil {
					log.Panic(err)
				} else if err = originIndexer.PostProcess(ctx, outpoint); err != nil {
					log.Panic(err)
				} else {
					log.Println("Processed", outpoint)
				}
			}
			if len(outpoints) == 0 {
				log.Println("No transactions to ingest")
				time.Sleep(time.Second)
			}
		}
	}
}
