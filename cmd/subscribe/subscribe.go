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
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var PORT int

var ctx = context.Background()
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

	var err error
	if err = lib.Initialize(); err != nil {
		log.Panic(err)
	}
}

func main() {
	var fromBlock uint64
	progressKey := lib.ProgressQueueKey(TAG)
	if progress, err := lib.Rdb.ZScore(ctx, progressKey, TOPIC).Result(); err == redis.Nil {
		fromBlock = uint64(FROM_BLOCK)
	} else if err != nil {
		log.Panic(err)
	} else {
		fromBlock = uint64(progress) - 5
	}

	log.Println("Subscribing to Junglebus from block", fromBlock)
	txfetch := make(chan string, 1000000)
	limiter := make(chan struct{}, 10)
	go func() {
		for txid := range txfetch {
			limiter <- struct{}{}
			go func(txid string) {
				defer func() {
					<-limiter
				}()

				if _, err := lib.LoadTx(ctx, txid, true); err != nil {
					log.Panic(err)
				}
			}(txid)
		}
	}()
	var sub *junglebus.Subscription
	// lastBlock := uint32(fromBlock)
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
			if err := lib.Queue.ZAdd(ctx, lib.IngestQueueKey, redis.Z{
				Score:  lib.HeightScore(txn.BlockHeight, txn.BlockIndex),
				Member: txn.Id,
			}).Err(); err != nil {
				log.Panic(err)
			}
			txfetch <- txn.Id
		}
	}
	if MEMOOOL {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			if VERBOSE > 0 {
				log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			}
			if err := lib.Queue.ZAdd(ctx, lib.IngestQueueKey, redis.Z{
				Score:  lib.HeightScore(uint32(time.Now().Unix()), 0),
				Member: txn.Id,
			}).Err(); err != nil {
				log.Panic(err)
			}
		}
	}

	var err error
	if sub, err = lib.JB.SubscribeWithQueue(ctx,
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
