package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "1sat"
const THREADS = 10

var db *sql.DB
var threadLimiter = make(chan struct{}, THREADS)

// var processedIdx = map[uint64]bool{}
var m sync.Mutex

func init() {
	godotenv.Load()

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Fatal(err)
	}

	err = lib.Initialize(db)
	if err != nil {
		log.Fatal(err)
	}
}

var wg sync.WaitGroup

// var txids = make(chan []byte, 2^32)
var junglebusClient *junglebus.JungleBusClient

// var txids = [][]byte{}
var sub *junglebus.Subscription
var fromBlock uint32
var settledHeight uint32

// var blockWg sync.WaitGroup

type TxnStatus struct {
	ID       string
	Tx       *bt.Tx
	Height   uint32
	Idx      uint32
	Parents  map[string]*TxnStatus
	Children map[string]*TxnStatus
}

var txns = map[string]*TxnStatus{}
var queue = make(chan *TxnStatus, 10000)
var settled = make(chan uint32)

func main() {
	var err error
	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP("https://junglebus.gorillapool.io"),
	)
	if err != nil {
		log.Fatalln(err.Error())
	}

	row := db.QueryRow(`SELECT height
			FROM progress
			WHERE indexer='1sat'`,
	)
	row.Scan(&fromBlock)
	if fromBlock < lib.TRIGGER {
		fromBlock = lib.TRIGGER
	}

	var wg2 sync.WaitGroup
	wg2.Add(1)
	go processInscriptionIds()
	subscribe()
	processTxns()
	wg2.Wait()
}

// var statusCode uint32

func subscribe() {
	var err error
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		os.Getenv("ONESAT"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: onTransactionHandler,
			// OnMempool:     onOneSatHandler,
			OnStatus: func(status *jbModels.ControlResponse) {
				switch status.StatusCode {
				case 1:
				case 2:
				case 10:
				case 11:
					log.Printf("[STATUS]: %v\n", status)
				case 200:
					log.Printf("[STATUS]: %v\n", status)
					sub.Unsubscribe()
					wg.Wait()
					settledHeight = status.Block - 6

					if _, err := db.Exec(`INSERT INTO progress(indexer, height)
						VALUES($1, $2)
						ON CONFLICT(indexer) DO UPDATE
							SET height=$2`,
						INDEXER,
						settledHeight,
					); err != nil {
						log.Print(err)
					}
					fromBlock = status.Block + 1
					// m.Lock()
					// processedIdx = map[uint64]bool{}
					// m.Unlock()
					// txids = [][]byte{}
					// go processInscriptionIds(settledHeight)
					fmt.Printf("Completed: %d\n", status.Block)
					settled <- settledHeight
					subscribe()
				case 300:
					log.Printf("[STATUS]: %v\n", status)
					sub.Unsubscribe()
					wg.Wait()
					fromBlock = status.Block
					subscribe()
				default:
					log.Panicf("[STATUS]: %v\n", status)
				}
			},
			OnError: func(err error) {
				log.Printf("[ERROR]: %v", err)
			},
		},
	)
	if err != nil {
		// log.Printf("ERROR: failed getting subscription %s", err.Error())

		// wg2.Done()
		log.Panic(err)
	}
}

func onTransactionHandler(txResp *jbModels.TransactionResponse) {
	fmt.Printf("[TX]: %d - %d: %s\n", txResp.BlockHeight, txResp.BlockIndex, txResp.Id)
	// if txResp.BlockHeight <= settledHeight {
	// 	fmt.Printf("Skipping: %s\n", txResp.Id)
	// 	return
	// }
	tx, err := bt.NewTxFromBytes(txResp.Transaction)
	if err != nil {
		log.Panicf("OnTransaction Parse Error: %s %+v\n", txResp.Id, err)
	}

	txn := &TxnStatus{
		ID:       txResp.Id,
		Tx:       tx,
		Height:   txResp.BlockHeight,
		Idx:      uint32(txResp.BlockIndex),
		Parents:  map[string]*TxnStatus{},
		Children: map[string]*TxnStatus{},
	}

	for _, input := range tx.Inputs {
		m.Lock()
		if parent, ok := txns[input.PreviousTxIDStr()]; ok {
			parent.Children[txResp.Id] = txn
			txn.Parents[parent.ID] = parent
		}
		m.Unlock()
	}
	wg.Add(1)
	m.Lock()
	txns[txResp.Id] = txn
	m.Unlock()

	if len(txn.Parents) == 0 {
		queue <- txn
	}
}

func processTxns() {
	for {
		txn := <-queue
		threadLimiter <- struct{}{}
		go func(txn *TxnStatus) {
			processTxn(txn)
			<-threadLimiter
		}(txn)
	}
}

func processTxn(txn *TxnStatus) {
	err := lib.IndexTxos(txn.Tx, txn.Height, txn.Idx)
	if err != nil {
		log.Fatal(err)
	}

	for _, child := range txn.Children {
		m.Lock()
		delete(child.Parents, txn.ID)
		orphan := len(child.Parents) == 0
		m.Unlock()
		if orphan {
			queue <- child
		}
	}
	m.Lock()
	delete(txns, txn.ID)
	m.Unlock()
	wg.Done()
}

// func processOrigins() {
// 	for _, txid := range txids {
// 		wg.Add(1)
// 		threadsChan <- struct{}{}
// 		go func(txid []byte) {
// 			err := lib.IndexOrigins(txid)
// 			if err != nil {
// 				log.Fatal(err)
// 			}
// 			wg.Done()
// 			<-threadsChan
// 		}(txid)
// 	}
// }

func processInscriptionIds() {
	for {
		height := <-settled
		fmt.Println("Processing inscription ids for height", height)
		err := lib.SetInscriptionIds(height)
		if err != nil {
			log.Panicln("Error processing inscription ids:", err)
		}
	}
}
