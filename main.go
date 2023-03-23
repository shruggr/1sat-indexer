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

var m sync.Mutex

func init() {
	godotenv.Load()

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}

	err = lib.Initialize(db)
	if err != nil {
		log.Panic(err)
	}
}

var wg sync.WaitGroup

var junglebusClient *junglebus.JungleBusClient

var sub *junglebus.Subscription
var fromBlock uint32
var settledHeight uint32

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
		log.Panicln(err.Error())
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
				log.Printf("[STATUS]: %v\n", status)
				switch status.StatusCode {
				case 200:
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
						log.Panic(err)
					}
					fromBlock = status.Block + 1
					fmt.Printf("Completed: %d\n", status.Block)
					settled <- settledHeight
					subscribe()
				case 300:
					sub.Unsubscribe()
					wg.Wait()
					fromBlock = status.Block
					subscribe()
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
	fmt.Printf("Processing: %s\n", txn.Tx.TxID())
	_, err := lib.IndexSpends(txn.Tx, true)
	if err != nil {
		log.Panic(err)
	}

	_, err = lib.IndexTxos(txn.Tx, txn.Height, txn.Idx, true)
	if err != nil {
		log.Panic(err)
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
