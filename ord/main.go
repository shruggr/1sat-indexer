package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "ord"

var THREADS uint64 = 16

var db *sql.DB
var junglebusClient *junglebus.Client

var sub *junglebus.Subscription
var threadLimiter = make(chan struct{}, THREADS)
var m sync.Mutex
var wg sync.WaitGroup
var txns = map[string]*TxnStatus{}
var msgQueue = make(chan *Msg, 100000)
var txnQueue = make(chan *TxnStatus, 100000)
var settled = make(chan uint32, 100)
var fromBlock uint32

var connected bool

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint32
	Transaction []byte
}

type TxnStatus struct {
	ID       string
	Tx       *bt.Tx
	Height   uint32
	Idx      uint32
	Parents  map[string]*TxnStatus
	Children map[string]*TxnStatus
}

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}

	err = lib.Initialize(db)
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
		junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

	row := db.QueryRow(`SELECT height
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
	var wg2 sync.WaitGroup
	wg2.Add(1)
	wg2.Wait()
}

func subscribe() {
	var err error
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		os.Getenv("ORD"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: func(txResp *jbModels.TransactionResponse) {
				log.Printf("[TX]: %d - %d: %s\n", txResp.BlockHeight, txResp.BlockIndex, txResp.Id)
				msgQueue <- &Msg{
					Id:          txResp.Id,
					Height:      txResp.BlockHeight,
					Idx:         uint32(txResp.BlockIndex),
					Transaction: txResp.Transaction,
				}
			},
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

				if status.StatusCode == 1 {
					if connected {
						sub.Unsubscribe()
						connected = false
						log.Printf("Cooling the Jets:")
						time.Sleep(30 * time.Second)
						subscribe()
					} else {
						connected = true
					}
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

func processQueue() {
	var settledHeight uint32
	go processInscriptionIds()
	go processTxns()
	for {
		msg := <-msgQueue

		switch msg.Status {
		case 0:
			tx, err := bt.NewTxFromBytes(msg.Transaction)
			if err != nil {
				if msg.Height == 0 {
					continue
				}
				log.Panicf("OnTransaction Parse Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
			}

			txn := &TxnStatus{
				ID:       msg.Id,
				Tx:       tx,
				Height:   msg.Height,
				Idx:      msg.Idx,
				Parents:  map[string]*TxnStatus{},
				Children: map[string]*TxnStatus{},
			}

			_, err = lib.SetTxn.Exec(msg.Id, msg.Hash, txn.Height, txn.Idx)
			if err != nil {
				panic(err)
			}

			if msg.Height == 0 {
				txnQueue <- txn
				continue
			}

			for _, input := range tx.Inputs {
				m.Lock()
				if parent, ok := txns[input.PreviousTxIDStr()]; ok {
					parent.Children[msg.Id] = txn
					txn.Parents[parent.ID] = parent
				}
				m.Unlock()
			}
			m.Lock()
			if t, ok := txns[msg.Id]; ok {
				t.Height = msg.Height
				t.Idx = msg.Idx
				m.Unlock()
				continue
			}
			txns[msg.Id] = txn
			m.Unlock()
			wg.Add(1)
			if len(txn.Parents) == 0 {
				txnQueue <- txn
			}
		// On Connected, if already connected, unsubscribe and cool down

		case 200:
			wg.Wait()
			// log.Panicf("Status: %d\n", msg.Status)
			settledHeight = msg.Height - 6

			if _, err := db.Exec(`INSERT INTO progress(indexer, height)
				VALUES($1, $2)
				ON CONFLICT(indexer) DO UPDATE
					SET height=$2`,
				INDEXER,
				settledHeight,
			); err != nil {
				log.Panic(err)
			}
			fromBlock = msg.Height + 1
			fmt.Printf("Completed: %d\n", msg.Height)
			settled <- settledHeight

		default:
			log.Printf("Status: %d\n", msg.Status)
		}
	}
}

func processTxns() {
	for {
		txn := <-txnQueue
		threadLimiter <- struct{}{}
		go func(txn *TxnStatus) {
			processTxn(txn)
			<-threadLimiter
		}(txn)
	}
}

func processTxn(txn *TxnStatus) {
	fmt.Printf("Processing: %d %d %s\n", txn.Height, txn.Idx, txn.Tx.TxID())
	_, err := lib.IndexSpends(txn.Tx, true)
	if err != nil {
		log.Panic(err)
	}

	_, err = lib.IndexTxos(txn.Tx, txn.Height, txn.Idx, true)
	if err != nil {
		log.Panic(err)
	}

	if txn.Height > 0 {
		for _, child := range txn.Children {
			m.Lock()
			delete(child.Parents, txn.ID)
			orphan := len(child.Parents) == 0
			m.Unlock()
			if orphan {
				txnQueue <- child
			}
		}
		m.Lock()
		delete(txns, txn.ID)
		m.Unlock()
		wg.Done()
	}
}

func processInscriptionIds() {
	for {
		height := <-settled
		fmt.Println("Processing inscription ids for height", height)
		err := lib.SetOriginNum(height)
		if err != nil {
			log.Panicln("Error processing inscription ids:", err)
		}
	}
}
