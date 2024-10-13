package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var PORT int

var ctx = context.Background()
var jb *junglebus.Client
var TOPIC string
var FROM_BLOCK uint
var VERBOSE int
var TAG string
var MEMOOOL bool
var BLOCK bool

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))
	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&MEMOOOL, "m", false, "Index Mempool")
	flag.BoolVar(&BLOCK, "b", true, "Index Blocks")
	flag.Parse()

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}

	log.Println("POSTGRES:", POSTGRES)
	var err error
	config, err := pgxpool.ParseConfig(POSTGRES)
	if err != nil {
		log.Panic(err)
	}
	config.MaxConnIdleTime = 15 * time.Second

	db, err := pgxpool.NewWithConfig(context.Background(), config)
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

	JUNGLEBUS := os.Getenv("JUNGLEBUS")
	if JUNGLEBUS == "" {
		JUNGLEBUS = "https://junglebus.gorillapool.io"
	}

	jb, err = junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panicln(err.Error())
	}
}

func main() {
	var fromBlock uint64
	progressKey := lib.ProgressQueueKey(TAG)
	ingestKey := lib.IngestQueueKey(TAG)
	if progress, err := lib.Rdb.ZScore(ctx, progressKey, TOPIC).Result(); err == redis.Nil {
		fromBlock = uint64(FROM_BLOCK)
	} else if err != nil {
		log.Panic(err)
	} else {
		fromBlock = uint64(progress) - 5
	}

	log.Println("Subscribing to Junglebus from block", fromBlock)

	var sub *junglebus.Subscription
	lastBlock := uint32(fromBlock)
	eventHandler := junglebus.EventHandler{
		OnStatus: func(status *models.ControlResponse) {
			// if VERBOSE > 0 {
			log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
			// }
			if status.StatusCode == 200 {
				if err := lib.Rdb.ZAdd(ctx, progressKey, redis.Z{
					Score:  float64(status.Block),
					Member: TOPIC,
				}).Err(); err != nil {
					log.Panic(err)
				}
			}
			if status.StatusCode == 999 {
				log.Println(status.Message)
				log.Println("Unsubscribing...")
				sub.Unsubscribe()
				os.Exit(0)
				return
			}
		},
		OnError: func(err error) {
			log.Panicf("[ERROR]: %v\n", err)
		},
	}
	if BLOCK {
		eventHandler.OnTransaction = func(txn *models.TransactionResponse) {
			if VERBOSE > 0 {
				log.Printf("[TX]: %d - %d: %d %s\n", txn.BlockHeight, txn.BlockIndex, len(txn.Transaction), txn.Id)
			}
			if err := lib.Rdb.ZAdd(ctx, ingestKey, redis.Z{
				Score:  lib.HeightScore(txn.BlockHeight, txn.BlockIndex),
				Member: txn.Id,
			}).Err(); err != nil {
				log.Panic(err)
			}
			if lastBlock != txn.BlockHeight {
				lastBlock = txn.BlockHeight
				if err := lib.Rdb.ZAdd(ctx, lib.ProgressQueueKey(TAG), redis.Z{
					Score:  float64(lastBlock),
					Member: TOPIC,
				}).Err(); err != nil {
					log.Panic(err)
				}
			}
		}
	}
	if MEMOOOL {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			if VERBOSE > 0 {
				log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			}
			if err := lib.Rdb.ZAdd(ctx, ingestKey, redis.Z{
				Score:  lib.HeightScore(uint32(time.Now().Unix()), 0),
				Member: txn.Id,
			}).Err(); err != nil {
				log.Panic(err)
			}
		}
	}

	var err error
	if sub, err = jb.SubscribeWithQueue(ctx,
		TOPIC,
		fromBlock,
		0,
		eventHandler,
		&junglebus.SubscribeOptions{
			QueueSize: 1000,
			LiteMode:  true,
		},
	); err != nil {
		log.Panic(err)
	}
	defer func() {
		sub.Unsubscribe()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Printf("Caught signal")
		fmt.Println("Unsubscribing and exiting...")
		sub.Unsubscribe()
		os.Exit(0)
	}()

	<-make(chan struct{})
}
