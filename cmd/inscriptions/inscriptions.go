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
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var POSTGRES string

var INDEXER string = "inscriptions"
var TOPIC string
var fromBlock uint
var VERBOSE int
var CONCURRENCY int = 64

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	// flag.StringVar(&INDEXER, "id", "inscriptions", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&fromBlock, "s", uint(lib.TRIGGER), "Start from block")
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
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

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

var ctx = context.Background()

func main() {
	errors := make(chan error)

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

	progress, err := rdb.HGet(ctx, "progress", INDEXER).Uint64()
	if err != nil && err != redis.Nil {
		panic(err)
	}

	if uint(progress) > fromBlock {
		fromBlock = uint(progress)
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

	log.Println("Subscribing to Junglebus from block", fromBlock)
	sub, err = junglebusClient.SubscribeWithQueue(
		context.Background(),
		TOPIC,
		uint64(fromBlock),
		0,
		junglebus.EventHandler{
			OnTransaction: func(txn *models.TransactionResponse) {
				txCtx, err := lib.ParseTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
				if err != nil {
					panic(err)
				}
				ordinals.ParseInscriptions(txCtx)
				for vout, txo := range txCtx.Txos {
					if _, ok := txo.Data["insc"]; !ok {
						continue
					}
					var num int
					row := lib.Db.QueryRow(context.Background(), `
						INSERT INTO inscriptions(height, idx, vout)
						VALUES($1, $2, $3)
						ON CONFLICT DO NOTHING
						RETURNING num`,
						txCtx.Height,
						txCtx.Idx,
						vout,
					)
					err = row.Scan(&num)
					if err == nil {
						log.Println("Inscription", num, "at", *txCtx.Height, txCtx.Idx, vout)
					}
				}

			},
			OnStatus: func(status *models.ControlResponse) {
				log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
				if status.StatusCode == 200 {
					fromBlock = uint(status.Block) - 1
					rdb.HSet(ctx, "progress", INDEXER, fromBlock)
				}
			},
			OnError: func(err error) {
				log.Printf("[ERROR]: %v\n", err)
				panic(err)
			},
		},
		1000000,
	)
	if err != nil {
		panic(err)
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

	<-errors
	sub.Unsubscribe()
}
