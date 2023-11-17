package junglebus

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

var Db *pgxpool.Pool
var Rdb *redis.Client
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

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
}

func Exec(
	indexBlocks bool,
	indexMempool bool,
	txHandler func(txn *lib.IndexContext) error,
	blockHander func(height uint32) error,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if sub != nil {
				sub.Unsubscribe()
			}
			fmt.Println("Recovered in f", r)
			fmt.Println("Unsubscribing and exiting...")
		}
	}()

	log.Println("POSTGRES:", POSTGRES)
	Db, err = pgxpool.New(context.Background(), POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	Rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(Db, Rdb)
	if err != nil {
		log.Panic(err)
	}

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
		if err != nil {
			log.Panic(err)
		}
	}

	JUNGLEBUS := os.Getenv("JUNGLEBUS")
	fmt.Println("JUNGLEBUS", JUNGLEBUS)

	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panicln(err.Error())
	}
	row := Db.QueryRow(context.Background(), `SELECT height
		FROM progress
		WHERE indexer=$1`,
		INDEXER,
	)
	err = row.Scan(&fromBlock)
	if err != nil {
		Db.Exec(context.Background(),
			`INSERT INTO progress(indexer, height)
				VALUES($1, 0)`,
			INDEXER,
		)
	}
	if fromBlock < lib.TRIGGER {
		fromBlock = lib.TRIGGER
	}

	eventHandler := junglebus.EventHandler{
		OnStatus: func(status *models.ControlResponse) {
			log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
			if status.StatusCode == 200 {
				wg.Wait()
				err = blockHander(status.Block)
				var settledHeight uint32
				if status.Block > 6 {
					settledHeight = status.Block - 6
				} else {
					settledHeight = 0
				}

				if _, err := Db.Exec(context.Background(),
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
		OnError: func(err error) {
			log.Printf("[ERROR]: %v", err)
		},
	}

	if indexBlocks {
		eventHandler.OnTransaction = func(txn *models.TransactionResponse) {
			log.Printf("[TX]: %d - %d: %d %s\n", txn.BlockHeight, txn.BlockIndex, len(txn.Transaction), txn.Id)
			threadLimiter <- struct{}{}
			wg.Add(1)
			Rdb.Set(context.Background(), txn.Id, txn.Transaction, 0).Err()
			go func(txn *models.TransactionResponse) {
				defer func() {
					<-threadLimiter
					wg.Done()
				}()
				txIndex, err := lib.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex, false)
				if err != nil {
					log.Panic(err)
				}
				if txHandler != nil {
					err = txHandler(txIndex)
					if err != nil {
						log.Panic(err)
					}
				}
			}(txn)
		}
	}
	if indexMempool {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			threadLimiter <- struct{}{}
			Rdb.Set(context.Background(), txn.Id, txn.Transaction, 0).Err()
			go func(txn *models.TransactionResponse) {
				defer func() {
					<-threadLimiter
				}()
				txIndex, err := lib.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex, false)
				if err != nil {
					log.Panic(err)
				}
				if txHandler != nil {
					err = txHandler(txIndex)
					if err != nil {
						log.Panic(err)
					}
				}
			}(txn)
		}
	}

	log.Println("Subscribing to Junglebus from block", fromBlock)
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		TOPIC,
		uint64(fromBlock),
		eventHandler,
	)
	if err != nil {
		log.Panic(err)
	}

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
	return
}
