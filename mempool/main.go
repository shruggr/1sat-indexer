package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
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

var THREADS uint64 = 16

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

var rdb *redis.Client

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES"),
	)
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

	go processQueue()
	// subscribe()
	go func() {
		sub := redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // no password set
			DB:       0,  // use default DB
		})
		ch := sub.Subscribe(context.Background(), "submit").Channel()
		for msg := range ch {
			txid := msg.Payload
			if len(txid) != 64 {
				continue
			}
			for i := 0; i < 4; i++ {
				tx, err := lib.LoadTx(txid)
				if err == nil {
					msgQueue <- &Msg{
						Id:          txid,
						Transaction: tx.Bytes(),
					}
					log.Printf("Indexing %s\n", txid)
					break
				}
				log.Printf("Retry %d: %s\n", i, txid)
				switch i {
				case 0:
					time.Sleep(2 * time.Second)
				case 1:
					time.Sleep(10 * time.Second)
				default:
					time.Sleep(30 * time.Second)
				}
			}

		}
	}()
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		sub.Unsubscribe()
	// 		fmt.Println("Recovered in f", r)
	// 		fmt.Println("Unsubscribing and exiting...")
	// 	}
	// }()

	// sigs := make(chan os.Signal, 1)
	// signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	// go func() {
	// 	<-sigs
	// 	fmt.Printf("Caught signal")
	// 	fmt.Println("Unsubscribing and exiting...")
	// 	sub.Unsubscribe()
	// 	os.Exit(0)
	// }()

	<-make(chan struct{})
}

func subscribe() {
	var err error
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		os.Getenv("MEMPOOL"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnMempool: func(txResp *jbModels.TransactionResponse) {
				if len(txResp.Transaction) == 0 {
					log.Printf("Empty Transaction: %v\n", txResp.Id)
				}
				// log.Printf("[MEMPOOL]: %v\n", txResp.Id)
				msgQueue <- &Msg{
					Id:          txResp.Id,
					Transaction: txResp.Transaction,
				}

			},
			OnStatus: func(status *jbModels.ControlResponse) {
				log.Printf("[STATUS]: %v\n", status)
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
		},
	)
	if err != nil {
		log.Panic(err)
	}
}

func processQueue() {
	go indexer.ProcessTxns(uint(THREADS))
	var err error
	mempoolCtx := &indexer.BlockCtx{
		StartTime: time.Now(),
	}
	for {
		msg := <-msgQueue

		var tx *bt.Tx
		if len(msg.Transaction) == 0 {
			tx, err = lib.LoadTx(msg.Id)
			if err != nil {
				log.Printf("OnTransaction Fetch Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
				continue
			}
			// msg.Transaction = txData.Transaction
			// continue
		} else {
			tx, err = bt.NewTxFromBytes(msg.Transaction)
			if err != nil {
				log.Printf("OnTransaction Parse Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
				continue
			}
		}

		txn := &indexer.TxnStatus{
			ID:       msg.Id,
			Tx:       tx,
			Parents:  map[string]*indexer.TxnStatus{},
			Children: map[string]*indexer.TxnStatus{},
			Ctx:      mempoolCtx,
		}

		indexer.TxnQueue <- txn
	}
}
