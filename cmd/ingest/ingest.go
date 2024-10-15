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

var limiter = make(chan struct{}, CONCURRENCY)
var m sync.Mutex

func main() {
	inflight := make(map[string]struct{}, CONCURRENCY)
	for {
		log.Println("Loading transactions to ingest")
		if txids, err := lib.Rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key:   lib.IngestQueueKey,
			Start: 0,
			Stop:  100,
		}).Result(); err != nil {
			log.Panic(err)
		} else {
			for _, txid := range txids {
				m.Lock()
				_, ok := inflight[txid]
				m.Unlock()
				if ok {
					continue
				}
				if children, err := lib.Cache.SMembers(ctx, lib.IngestDepsKey(txid)).Result(); err != nil {
					log.Panic(err)
				} else if len(children) > 0 {
					hasDeps := false
					for _, child := range children {
						if err := lib.Rdb.ZScore(ctx, lib.IngestLogKey, child).Err(); err == redis.Nil {
							hasDeps = true
						} else if err != nil {
							log.Panic(err)
						} else {
							lib.Rdb.ZRem(ctx, lib.IngestQueueKey, child)
						}
					}

					if hasDeps {
						return
					}
				}
				limiter <- struct{}{}
				m.Lock()
				inflight[txid] = struct{}{}
				m.Unlock()
				go func(txid string) {
					defer func() {
						m.Lock()
						delete(inflight, txid)
						m.Unlock()
						<-limiter
					}()

					if _, err := lib.IngestTxid(ctx, txid, indexers); err != nil {
						log.Panic(err)
					}
				}(txid)
			}
			if len(txids) == 0 {
				log.Println("No transactions to ingest")
				time.Sleep(time.Second)
			}
		}
	}
}
