package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
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

const INDEXER = "1sat"

var THREADS uint64 = 64

var db *pgxpool.Pool
var junglebusClient *junglebus.Client
var msgQueue = make(chan *Msg, 100000)
var settle = make(chan uint32, 1000)
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
	godotenv.Load("../.env")

	var err error
	log.Println("POSTGRES:", os.Getenv("POSTGRES_FULL"))
	db, err = pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
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
	fmt.Println("JUNGLEBUS", os.Getenv("JUNGLEBUS"))
	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	)
	if err != nil {
		log.Panicln(err.Error())
	}
	row := db.QueryRow(context.Background(), `SELECT height
		FROM progress
		WHERE indexer=$1`,
		INDEXER,
	)
	row.Scan(&fromBlock)
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

	var wg2 sync.WaitGroup
	wg2.Add(1)
	wg2.Wait()
}

func subscribe() {
	var err error
	log.Println("Subscribing to Junglebus from block", fromBlock)
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		os.Getenv("ONESAT"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: func(tx *jbModels.TransactionResponse) {
				log.Printf("[TX]: %d - %d: %d %s\n", tx.BlockHeight, tx.BlockIndex, len(tx.Transaction), tx.Id)
				msgQueue <- &Msg{
					Id:          tx.Id,
					Height:      tx.BlockHeight,
					Idx:         tx.BlockIndex,
					Transaction: tx.Transaction,
				}
			},
			OnStatus: func(status *jbModels.ControlResponse) {
				log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
				if status.StatusCode == 999 {
					log.Println(status.Message)
					log.Println("Unsubscribing...")
					sub.Unsubscribe()
					log.Println("Unsubscribed")
					os.Exit(0)
					return
				}

				msgQueue <- &Msg{
					Height: status.Block,
					Status: status.StatusCode,
				}
			},
			// OnMempool: func(tx *jbModels.TransactionResponse) {
			// 	log.Printf("[MEMPOOL]: %d %s\n", len(tx.Transaction), tx.Id)
			// 	msgQueue <- &Msg{
			// 		Id:          tx.Id,
			// 		Transaction: tx.Transaction,
			// 	}
			// },
			OnError: func(err error) {
				log.Panicf("[ERROR]: %v", err)
			},
		},
	)
	if err != nil {
		log.Panic(err)
	}
}

func processQueue() {
	// var settledHeight uint32
	go processCompletions()
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

			rdb.Set(context.Background(), txn.ID, txn.Tx.Bytes(), 0).Err()

			indexer.M.Lock()
			_, ok := indexer.Txns[msg.Id]
			indexer.M.Unlock()
			if ok {
				continue
			}
			indexer.M.Lock()
			indexer.Txns[txn.ID] = txn
			for _, input := range txn.Tx.Inputs {
				inTxid := input.PreviousTxIDStr()
				if parent, ok := indexer.Txns[inTxid]; ok {
					parent.Children[msg.Id] = txn
					txn.Parents[parent.ID] = parent
				}
			}
			indexer.M.Unlock()

			// indexer.M.Lock()
			// indexer.Txns[msg.Id] = txn
			// indexer.M.Unlock()

			if len(txn.Parents) == 0 {
				indexer.Wg.Add(1)
				indexer.InQueue++
				indexer.TxnQueue <- txn
			}

		case 200:
			indexer.Wg.Wait()
			rdb.Publish(context.Background(), "indexed", fmt.Sprintf("%d", msg.Height))

			if _, err := db.Exec(context.Background(),
				`UPDATE progress
					SET height=$2
					WHERE indexer=$1 and height<$2`,
				INDEXER,
				msg.Height-6,
			); err != nil {
				log.Panic(err)
			}
			fromBlock = msg.Height + 1
			// fmt.Printf("Completed: %d\n", msg.Height)
			settle <- msg.Height

		default:
			log.Printf("Status: %d\n", msg.Status)
		}
	}
}

func processCompletions() {
	for height := range settle {
		var settled uint32
		if height > 6 {
			settled = height - 6
		}
		// if _, err := db.Exec(context.Background(), `
		// 	INSERT INTO progress(indexer, height)
		// 	VALUES($1, $2)
		// 	ON CONFLICT(indexer) DO UPDATE
		// 		SET height=$2
		// 		WHERE progress.height < $2`,
		// 	INDEXER,
		// 	settled,
		// ); err != nil {
		// 	log.Panic(err)
		// }

		var wg sync.WaitGroup
		wg.Add(2)
		fmt.Println("Processing completions for height", height)
		go func(height uint32) {
			// fmt.Println("Processing inscription ids for height", height)
			err := lib.SetOriginNum(height)
			if err != nil {
				log.Panicln("Error processing inscription ids:", err)
			}
			wg.Done()
		}(settled)

		go func(height uint32) {
			// fmt.Println("Validating bsv20 for height", height)
			lib.ValidateBsv20(height)
			wg.Done()
		}(height)

		wg.Wait()
		// fmt.Println("Done processing inscription ids")
		// rdb.Publish(context.Background(), "settled", fmt.Sprintf("%d", height))
	}
}
