package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
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

	flag.StringVar(&INDEXER, "idx", "", "Indexer name")
	flag.StringVar(&POSTGRES, "pg", "", "Postgres connection string")
	flag.StringVar(&TOPIC, "topic", "", "Junglebus topic")

	flag.Parse()
	var err error

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
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
	JUNGLEBUS := "https://prod.junglebus.gorillapool.io" // os.Getenv("JUNGLEBUS")
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

	go processQueue()
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
			OnTransaction: func(txResp *jbModels.TransactionResponse) {
				log.Printf("[TX]: %d - %d: %d %s\n", txResp.BlockHeight, txResp.BlockIndex, len(txResp.Transaction), txResp.Id)
				msgQueue <- &Msg{
					Id:          txResp.Id,
					Height:      txResp.BlockHeight,
					Idx:         txResp.BlockIndex,
					Transaction: txResp.Transaction,
				}
			},
			OnStatus: func(status *jbModels.ControlResponse) {
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
			OnMempool: func(tx *jbModels.TransactionResponse) {
				log.Printf("[MEMPOOL]: %d %s\n", len(tx.Transaction), tx.Id)
				msgQueue <- &Msg{
					Id:          tx.Id,
					Transaction: tx.Transaction,
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

func processQueue() {
	var settledHeight uint32
	go indexer.ProcessTxns(uint(THREADS))
	for {
		msg := <-msgQueue

		switch msg.Status {
		case 0:
			tx, err := bt.NewTxFromBytes(msg.Transaction)
			if err != nil {
				log.Panicf("OnTransaction Parse Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
			}

			txn := &indexer.TxnStatus{
				ID:       msg.Id,
				Tx:       tx,
				Height:   &msg.Height,
				Idx:      msg.Idx,
				Parents:  map[string]*indexer.TxnStatus{},
				Children: map[string]*indexer.TxnStatus{},
				Ctx: &indexer.BlockCtx{
					Hash:      msg.Hash,
					Height:    &msg.Height,
					StartTime: time.Now(),
				},
			}

			indexer.M.Lock()
			_, ok := indexer.Txns[msg.Id]
			indexer.M.Unlock()
			if ok {
				continue
			}
			for _, input := range tx.Inputs {
				inTxid := input.PreviousTxIDStr()
				indexer.M.Lock()
				if parent, ok := indexer.Txns[inTxid]; ok {
					parent.Children[msg.Id] = txn
					txn.Parents[parent.ID] = parent
				}
				indexer.M.Unlock()
			}
			indexer.M.Lock()
			indexer.Txns[msg.Id] = txn
			indexer.M.Unlock()
			if len(txn.Parents) == 0 {
				indexer.Wg.Add(1)
				indexer.InQueue++
				indexer.TxnQueue <- txn
			}
		// On Connected, if already connected, unsubscribe and cool down

		case 200:
			indexer.Wg.Wait()
			rdb.Publish(context.Background(), "indexed", fmt.Sprintf("%d", msg.Height-1))
			if msg.Height > 6 {
				settledHeight = msg.Height - 6
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
			fromBlock = msg.Height + 1

		default:
			log.Printf("Status: %d\n", msg.Status)
		}
	}
}
