package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/go-block-headers-client/client"
)

var POSTGRES string
var INDEXER string = "bsv20"
var TOPIC string
var fromBlock uint
var CONCURRENCY int = 64
var headers *client.HeadersClient

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	// flag.StringVar(&INDEXER, "id", "inscriptions", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&fromBlock, "s", uint(lib.TRIGGER), "Start from block")
	flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	// flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	var err error
	log.Println("POSTGRES:", POSTGRES)
	db, err := pgxpool.New(context.Background(), POSTGRES)
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

	err = lib.Initialize(db, rdb, cache)
	if err != nil {
		log.Panic(err)
	}

	headers = &client.HeadersClient{
		Url: os.Getenv("HEADERS_URL"),
	}
}

func main() {
	ctx := context.Background()
	JUNGLEBUS := os.Getenv("JUNGLEBUS")
	if JUNGLEBUS == "" {
		JUNGLEBUS = "https://junglebus.gorillapool.io"
	}
	fmt.Println("JUNGLEBUS", JUNGLEBUS, TOPIC)

	junglebusClient, err := junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

	row := lib.Db.QueryRow(context.Background(), `
		SELECT height
		FROM progress
		WHERE indexer='bsv20'`,
	)
	var lastProcessed uint32
	err = row.Scan(&lastProcessed)
	if err != nil {
		log.Panicln(err.Error())
	}
	if lastProcessed < uint32(fromBlock) {
		lastProcessed = uint32(fromBlock)
	}

	var txCount int
	var height uint32
	var idx uint64
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			if txCount > 0 {
				log.Printf("Blk %d I %d - %d txs %d/s\n", height, idx, txCount, txCount/10)
			}
			txCount = 0
		}
	}()

	var sub *junglebus.Subscription
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Printf("Caught signal")
		fmt.Println("Unsubscribing and exiting...")
		if sub != nil {
			sub.Unsubscribe()
		}
		os.Exit(0)
	}()

	var chaintip *client.BlockHeader
	timer := time.NewTicker(30 * time.Second)
	for {
		<-timer.C
		if sub != nil {
			continue
		}
		if chaintip, err = headers.GetChaintip(ctx); err != nil {
			log.Panicln("Error getting chaintip:", err)
		}

		if chaintip.Height < lastProcessed+5 {
			log.Println("Waiting for chaintip", lastProcessed+5, chaintip.Height)
			continue
		}

		log.Println("Subscribing to Junglebus from block", lastProcessed)
		var wg sync.WaitGroup
		limiter := make(chan struct{}, CONCURRENCY)
		sub, err = junglebusClient.SubscribeWithQueue(
			context.Background(),
			TOPIC,
			uint64(lastProcessed),
			0,
			junglebus.EventHandler{
				OnTransaction: func(txn *models.TransactionResponse) {
					limiter <- struct{}{}
					wg.Add(1)
					go func(txn *models.TransactionResponse) {
						defer func() {
							<-limiter
							wg.Done()
						}()
						txCtx, err := lib.ParseTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
						if err != nil {
							panic(err)
						}
						ordinals.ParseInscriptions(txCtx)
						ticks := map[string]uint64{}
						for _, txo := range txCtx.Txos {
							if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
								ticker := bsv20.Ticker
								if ticker == "" {
									if bsv20.Id == nil {
										continue
									}
									ticker = bsv20.Id.String()
								}
								if txouts, ok := ticks[ticker]; !ok {
									ticks[ticker] = 1
								} else {
									ticks[ticker] = txouts + 1
								}
							}
						}
						for ticker, txouts := range ticks {
							var result pgconn.CommandTag
							id, err := lib.NewOutpointFromString(ticker)
							if err != nil {
								result, err = lib.Db.Exec(context.Background(), `
									INSERT INTO bsv20v1_txns(txid, tick, height, idx, txouts)
									VALUES($1, $2, $3, $4, $5)
									ON CONFLICT(txid, tick) DO NOTHING`,
									txCtx.Txid,
									ticker,
									txCtx.Height,
									txCtx.Idx,
									txouts,
								)
							} else {
								result, err = lib.Db.Exec(context.Background(), `
									INSERT INTO bsv20v2_txns(txid, id, height, idx, txouts)
									VALUES($1, $2, $3, $4, $5)
									ON CONFLICT(txid, id) DO NOTHING`,
									txCtx.Txid,
									id,
									txCtx.Height,
									txCtx.Idx,
									txouts,
								)
							}
							if err != nil {
								log.Panicln(err)
							} else if result.RowsAffected() > 0 {
								log.Println("Inserted", ticker, hex.EncodeToString(txCtx.Txid))
							}
						}
					}(txn)

				},
				OnStatus: func(status *models.ControlResponse) {
					log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
					if status.StatusCode == 200 {
						wg.Wait()
						lastProcessed = status.Block
						lib.Db.Exec(context.Background(),
							`UPDATE progress
								SET height=$1
								WHERE indexer='bsv20' and height<$1`,
							lastProcessed,
						)
						if status.Block > chaintip.Height-5 {
							log.Println("Caught up to chaintip - 5. Waiting", status.Block, chaintip.Height)
							sub.Unsubscribe()
							sub = nil
							return
						}
					}
				},
				OnError: func(err error) {
					log.Printf("[ERROR]: %v\n", err)
					panic(err)
				},
			},
			&junglebus.SubscribeOptions{
				QueueSize: 100000,
			},
		)
		if err != nil {
			panic(err)
		}
	}
}
