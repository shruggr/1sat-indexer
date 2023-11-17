package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var INDEXER string
var TOPIC string
var THREADS uint64 = 64

var db *pgxpool.Pool
var rdb *redis.Client
var junglebusClient *junglebus.Client
var threadLimiter = make(chan struct{}, THREADS)
var fromBlock uint32
var sub *junglebus.Subscription

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint64
	Transaction []byte
}

// var txnQueue = make(chan *models.Transaction, 1000000)
// var m sync.Mutex
var wg sync.WaitGroup

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../.env`, wd))

	flag.StringVar(&INDEXER, "idx", "", "Indexer name")
	flag.StringVar(&POSTGRES, "pg", "", "Postgres connection string")
	flag.StringVar(&TOPIC, "topic", "", "Junglebus topic")

	flag.Parse()
	var err error

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	log.Println("POSTGRES:", POSTGRES)
	db, err = pgxpool.New(context.Background(), POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
		if err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	var err error
	JUNGLEBUS := os.Getenv("JUNGLEBUS")
	fmt.Println("JUNGLEBUS", JUNGLEBUS)

	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panicln(err.Error())
	}
	row := db.QueryRow(context.Background(), `SELECT height
		FROM progress
		WHERE indexer=$1`,
		INDEXER,
	)
	err = row.Scan(&fromBlock)
	if err != nil {
		db.Exec(context.Background(),
			`INSERT INTO progress(indexer, height)
				VALUES($1, 0)`,
			INDEXER,
		)
	}
	if fromBlock < lib.TRIGGER {
		fromBlock = lib.TRIGGER
	}

	subscribe()
	defer func() {
		if r := recover(); r != nil {
			sub.Unsubscribe()
			fmt.Println("Recovered in f", r)
			fmt.Println("Unsubscribing and exiting...")
		}
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

func subscribe() {
	var err error
	log.Println("Subscribing to Junglebus from block", fromBlock)
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		TOPIC,
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: func(txn *models.TransactionResponse) {
				log.Printf("[TX]: %d - %d: %d %s\n", txn.BlockHeight, txn.BlockIndex, len(txn.Transaction), txn.Id)
				threadLimiter <- struct{}{}
				wg.Add(1)
				rdb.Set(context.Background(), txn.Id, txn.Transaction, 0).Err()
				go func(txn *models.TransactionResponse) {
					defer func() {
						<-threadLimiter
						wg.Done()
					}()
					lib.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex, false)
				}(txn)

			},
			OnStatus: func(status *models.ControlResponse) {
				log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
				if status.StatusCode == 200 {
					wg.Wait()
					var settledHeight uint32
					if status.Block > 6 {
						settledHeight = status.Block - 6
					} else {
						settledHeight = 0
					}

					if _, err := db.Exec(context.Background(),
						`UPDATE progress
						SET height=$2
						WHERE indexer=$1 and height<$2`,
						INDEXER,
						settledHeight,
					); err != nil {
						log.Panic(err)
					}
					fromBlock = status.Block + 1
				}
				if status.StatusCode == 999 {
					log.Println(status.Message)
					log.Println("Unsubscribing...")
					sub.Unsubscribe()
					os.Exit(0)
					return
				}
			},
			// OnMempool: func(tx *models.TransactionResponse) {
			// 	log.Printf("[MEMPOOL]: %d %s\n", len(tx.Transaction), tx.Id)
			// 	msgQueue <- &Msg{
			// 		Id:          tx.Id,
			// 		Transaction: tx.Transaction,
			// 	}
			// },
			OnError: func(err error) {
				log.Printf("[ERROR]: %v", err)
			},
		},
	)
	if err != nil {
		log.Panic(err)
	}
}
