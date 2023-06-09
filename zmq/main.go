package main

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/GorillaPool/go-junglebus"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

var THREADS uint64 = 16

var db *sql.DB
var junglebusClient *junglebus.Client
var msgQueue = make(chan *Msg, 1000000)
var fromBlock uint32
var sub *junglebus.Subscription
var bit *bitcoin.Bitcoind

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
	port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	if err != nil {
		log.Panic(err)
	}
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
	// var err error
	// fmt.Println("JUNGLEBUS", os.Getenv("JUNGLEBUS"))
	// junglebusClient, err = junglebus.New(
	// 	junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	// )
	// if err != nil {
	// 	log.Panicln(err.Error())
	// }

	port, _ := strconv.ParseInt(os.Getenv("ZMQ_PORT"), 10, 32)
	zmq := bitcoin.NewZMQ(os.Getenv("BITCOIN_HOST"), int(port))

	ch := make(chan []string)

	go func() {
		for c := range ch {
			log.Printf("%v", c)
		}
	}()

	if err := zmq.Subscribe("rawtx", ch); err != nil {
		log.Fatalf("%v", err)
	}

	if err := zmq.Subscribe("hashblock", ch); err != nil {
		log.Fatalf("%v", err)
	}

	waitCh := make(chan bool)
	<-waitCh

	go processQueue()
	// subscribe()
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

func processQueue() {
	go indexer.ProcessTxns(uint(THREADS))
	for {
		msg := <-msgQueue

		if len(msg.Transaction) == 0 {
			txid, err := hex.DecodeString(msg.Id)
			if err != nil {
				log.Printf("OnTransaction Hex Decode Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
				continue
			}
			txData, err := lib.LoadTxData(txid)
			if err != nil {
				log.Printf("OnTransaction Fetch Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
				continue
			}
			msg.Transaction = txData.Transaction
		}

		tx, err := bt.NewTxFromBytes(msg.Transaction)
		if err != nil {
			log.Printf("OnTransaction Parse Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
			continue
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
