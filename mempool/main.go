package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

var THREADS uint64 = 16

var db *sql.DB
var junglebusClient *junglebus.Client
var msgQueue = make(chan *Msg, 1000000)
var settled = make(chan uint32, 100)
var fromBlock uint32

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint32
	Transaction []byte
}

var rdb *redis.Client

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
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
	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP("https://junglebus.gorillapool.io"),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

	go processQueue()
	subscribe()
	var wg2 sync.WaitGroup
	wg2.Add(1)
	wg2.Wait()
}

func subscribe() {
	var err error
	_, err = junglebusClient.Subscribe(
		context.Background(),
		os.Getenv("MEMPOOL"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnMempool: func(txResp *jbModels.TransactionResponse) {
				log.Printf("[MEMPOOL]: %v\n", txResp.Id)
				msgQueue <- &Msg{
					Id:          txResp.Id,
					Height:      txResp.BlockHeight,
					Idx:         uint32(txResp.BlockIndex),
					Transaction: txResp.Transaction,
				}

			},
			OnStatus: func(status *jbModels.ControlResponse) {
				log.Printf("[STATUS]: %v\n", status)
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
	go indexer.ProcessInscriptionIds(settled)
	go indexer.ProcessTxns(uint(THREADS))
	for {
		msg := <-msgQueue

		tx, err := bt.NewTxFromBytes(msg.Transaction)
		if err != nil {
			if msg.Height == 0 {
				continue
			}
			log.Panicf("OnTransaction Parse Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
		}

		txn := &indexer.TxnStatus{
			ID:       msg.Id,
			Tx:       tx,
			Height:   msg.Height,
			Idx:      msg.Idx,
			Parents:  map[string]*indexer.TxnStatus{},
			Children: map[string]*indexer.TxnStatus{},
		}

		_, err = lib.SetTxn.Exec(msg.Id, msg.Hash, txn.Height, txn.Idx)
		if err != nil {
			panic(err)
		}

		indexer.TxnQueue <- txn
	}
}
