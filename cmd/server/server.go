package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/ingest"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/server"
)

var PORT int
var CONCURRENCY uint
var VERBOSE int
var RUN_INGEST bool
var RUN_OWNERS bool

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(".env")

	PORT, _ = strconv.Atoi(os.Getenv("PORT"))
	flag.IntVar(&PORT, "p", PORT, "Port to listen on")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&RUN_INGEST, "ingest", false, "Run ingest service")
	flag.BoolVar(&RUN_OWNERS, "owners", false, "Run owners sync service")
	flag.Parse()
}

func main() {
	ctx := context.Background()

	ingestCtx := &idx.IngestCtx{
		Tag:         idx.IngestTag,
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
		Store:       config.Store,
		Verbose:     VERBOSE > 0,
	}

	// Start ingest service if requested
	if RUN_INGEST {
		var redisClient *redis.Client
		if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
			log.Printf("Error parsing REDISEVT URL: %v", err)
		} else {
			redisClient = redis.NewClient(opts)
		}

		ingestCtx.Key = idx.QueueKey(idx.IngestTag)
		ingestCtx.PageSize = 1000

		go ingest.Start(ctx, ingestCtx, config.Broadcaster, redisClient, false)
		log.Println("Started ingest service")
	}

	// Start owners sync service if requested
	if RUN_OWNERS {
		go runOwnersSync(ctx)
		log.Println("Started owners sync service")
	}

	ingestCtx.Once = true
	app := server.Initialize(ingestCtx, config.Broadcaster)
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}

func runOwnersSync(ctx context.Context) {
	store := config.Store
	for {
		if results, err := store.Search(ctx, &idx.SearchCfg{
			Keys: []string{idx.OwnerSyncKey},
		}); err != nil {
			log.Panic(err)
		} else if len(results) == 0 {
			time.Sleep(time.Minute)
			continue
		} else {
		addLoop:
			for _, result := range results {
				lastHeight := int(result.Score)
				for {
					log.Println("Owner", result.Member, "lastHeight", lastHeight)
					start := time.Now()
					if addTxns, err := jb.FetchOwnerTxns(result.Member, lastHeight); err == jb.ErrBadRequest {
						log.Println("ErrBadRequest", result.Member)
						continue addLoop
					} else if err != nil {
						log.Panic(err)
					} else {
						log.Println("Fetched", len(addTxns), "txns in", time.Since(start))
						for _, addTxn := range addTxns {
							if addTxn.Height > uint32(lastHeight) {
								lastHeight = int(addTxn.Height)
							}
							if score, err := store.LogScore(ctx, idx.LogKey(idx.IngestTag), addTxn.Txid); err != nil && err != redis.Nil {
								log.Panic(err)
							} else if score > 0 && score <= idx.MempoolScore {
								log.Println("Skipping", addTxn.Txid, score)
								continue
							}
							score := idx.HeightScore(addTxn.Height, addTxn.Idx)
							if logged, err := store.LogOnce(ctx, idx.QueueKey(idx.IngestTag), addTxn.Txid, score); err != nil {
								log.Panic(err)
							} else if logged {
								log.Println("Queuing", addTxn.Txid, score)
							} else {
								log.Println("Skipping", addTxn.Txid, score)
							}
						}

						if err := store.Log(ctx, idx.OwnerSyncKey, result.Member, float64(lastHeight)); err != nil {
							log.Panic(err)
						}
						if len(addTxns) < 1000 {
							break
						}
						lastHeight++
					}
				}
			}
		}
		time.Sleep(10 * time.Minute)
	}
}
