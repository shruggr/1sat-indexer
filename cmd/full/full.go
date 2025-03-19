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
	"github.com/shruggr/1sat-indexer/v5/audit"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/server"
)

var PORT int
var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string
var ancestorConfig = idx.AncestorConfig{
	Parse: true,
}

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	PORT, _ = strconv.Atoi(os.Getenv("PORT"))
	flag.IntVar(&PORT, "p", PORT, "Port to listen on")
	flag.StringVar(&TAG, "tag", idx.IngestTag, "Log tag")
	flag.StringVar(&QUEUE, "q", idx.IngestTag, "Queue tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	// flag.BoolVar(&ancestorConfig.Load, "l", false, "Load ancestors")
	// flag.BoolVar(&ancestorConfig.Parse, "p", true, "Parse ancestors")
	// flag.BoolVar(&ancestorConfig.Save, "s", false, "Save ancestors")
	flag.Parse()
	log.Println(ancestorConfig)
}

func main() {
	ctx := context.Background()
	ingest := &idx.IngestCtx{
		Tag:            TAG,
		Key:            idx.QueueKey(QUEUE),
		Indexers:       config.Indexers,
		Network:        config.Network,
		Concurrency:    CONCURRENCY,
		Once:           true,
		Store:          config.Store,
		PageSize:       1000,
		AncestorConfig: ancestorConfig,
		Verbose:        VERBOSE > 0,
	}

	go func() {
		if err := (ingest).Exec(ctx); err != nil {
			log.Println("Ingest error", err)
		}
	}()

	go func() {
		for {
			if results, err := config.Store.Search(ctx, &idx.SearchCfg{
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
						if VERBOSE > 0 {
							log.Println("Owner", result.Member, "lastHeight", lastHeight)
						}
						// start := time.Now()
						if addTxns, err := jb.FetchOwnerTxns(result.Member, lastHeight); err == jb.ErrBadRequest {
							log.Println("ErrBadRequest", result.Member)
							continue addLoop
						} else if err != nil {
							log.Panic(err)
						} else {
							// log.Println("Fetched", len(addTxns), "txns in", time.Since(start))
							for _, addTxn := range addTxns {
								if addTxn.Height > uint32(lastHeight) {
									lastHeight = int(addTxn.Height)
								}
								if score, err := config.Store.LogScore(ctx, idx.LogKey(TAG), addTxn.Txid); err != nil && err != redis.Nil {
									log.Panic(err)
								} else if score > 0 && score <= idx.MempoolScore {
									if VERBOSE > 0 {
										log.Println("Skipping", addTxn.Txid, score)
									}
									continue
								}
								score := idx.HeightScore(addTxn.Height, addTxn.Idx)
								if logged, err := config.Store.LogOnce(ctx, idx.QueueKey(TAG), addTxn.Txid, score); err != nil {
									log.Panic(err)
								} else if logged && VERBOSE > 0 {
									log.Println("Queuing", addTxn.Txid, score)
								} else if VERBOSE > 0 {
									log.Println("Skipping", addTxn.Txid, score)
								}
							}

							if err := config.Store.Log(ctx, idx.OwnerSyncKey, result.Member, float64(lastHeight)); err != nil {
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
	}()

	go func() {
		audit.StartTxAudit(context.Background(), &idx.IngestCtx{
			Indexers: config.Indexers,
			Network:  config.Network,
			Store:    config.Store,
		}, config.Broadcaster, true)
	}()

	app := server.Initialize(&idx.IngestCtx{
		Tag:         idx.IngestTag,
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
		Once:        true,
		Store:       config.Store,
		// Verbose:     VERBOSE > 0,
		Verbose: true,
	}, config.Broadcaster)
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}
