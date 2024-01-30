package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var INDEXER string
var TOPIC string
var THREADS uint64 = 64

var db *pgxpool.Pool
var junglebusClient *junglebus.Client
var msgQueue = make(chan *Msg, 1000000)
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

// var ctx = context.Background()
var rdb *redis.Client

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../.env`, wd))

	flag.StringVar(&INDEXER, "idx", "op_codes", "Indexer name")
	flag.StringVar(&POSTGRES, "pg", "", "Postgres connection string")
	flag.StringVar(&TOPIC, "topic", "9835d9e47565c24d847e5e88c770f9dd306031861d8cacebfe4f8124e712a5bf", "Junglebus topic")

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
		Addr:     os.Getenv("REDIS"),
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
	if fromBlock == 0 {
		fromBlock = 1
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
	opCodes := map[byte]uint64{}

	var wg sync.WaitGroup
	txChan := make(chan *map[byte]uint64, 10000)
	go func() {
		for txOps := range txChan {
			for op, count := range *txOps {
				if _, ok := opCodes[op]; !ok {
					opCodes[op] = 0
				}
				opCodes[op] += count
			}
			wg.Done()
		}
	}()
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		TOPIC,
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: func(txResp *jbModels.TransactionResponse) {
				log.Printf("[TX]: %d - %d: %d %s\n", txResp.BlockHeight, txResp.BlockIndex, len(txResp.Transaction), txResp.Id)
				wg.Add(1)

				go func(rawtx []byte) {
					tx, err := bt.NewTxFromBytes(rawtx)
					if err != nil {
						log.Panicf("OnTransaction Parse Error: %s %d %+v\n", txResp.Id, len(txResp.Transaction), err)
					}
					txOps := map[byte]uint64{}
				outputsLoop:
					for _, out := range tx.Outputs {
						var pos int
						for pos < len(*out.LockingScript) {
							op, err := lib.ReadOp(*out.LockingScript, &pos)
							if err != nil {
								continue outputsLoop
							}
							if _, ok := txOps[op.OpCode]; !ok {
								txOps[op.OpCode] = 0
							}
							txOps[op.OpCode]++
						}
					}
					txChan <- &txOps
				}(txResp.Transaction)
			},
			OnStatus: func(status *jbModels.ControlResponse) {
				log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
				if status.StatusCode == 200 {
					wg.Wait()
					if len(opCodes) > 0 {
						out, err := json.Marshal(opCodes)
						if err != nil {
							log.Panic(err)
						}
						os.WriteFile(fmt.Sprintf("data/%06d.json", status.Block), out, 0644)
					}
					opCodes = map[byte]uint64{}
					if _, err := db.Exec(context.Background(),
						`UPDATE progress
							SET height=$2
							WHERE indexer=$1 and height<$2`,
						INDEXER,
						status.Block,
					); err != nil {
						log.Panic(err)
					}

				}
				// log.Printf("[STATUS]: %v\n", status)
				if status.StatusCode == 999 {
					log.Println(status.Message)
					log.Println("Unsubscribing...")
					sub.Unsubscribe()
					os.Exit(0)
					return
				}
				msgQueue <- &Msg{
					Height: status.Block,
					Status: status.StatusCode,
				}
			},
			OnError: func(err error) {
				log.Printf("[ERROR]: %v", err)
			},
		},
	)
	if err != nil {
		log.Panic(err)
	}
}
