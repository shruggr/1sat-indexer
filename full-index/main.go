package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/libsv/go-bt/v2"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/tikv/client-go/v2/txnkv"
)

var THREADS uint64 = 15

var Tikv *txnkv.Client
var jbClient *junglebus.Client
var threadLimiter = make(chan struct{}, THREADS)
var m sync.Mutex
var wg sync.WaitGroup
var txns = map[string]*lib.TxnStatus{}
var msgQueue = make(chan *lib.Msg, 100000)
var txnQueue = make(chan *lib.TxnStatus, 100000)
var settled = make(chan uint32, 100)
var fromBlock uint32

func init() {
	var err error
	Tikv, err = txnkv.NewClient([]string{os.Getenv("TIKV")})
	if err != nil {
		log.Panicln(err.Error())
	}

	jbClient, err = junglebus.New(
		junglebus.WithHTTP("https://junglebus.gorillapool.io"),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

	buf, err := os.ReadFile("last_block")
	if err != nil {
		from, _ := strconv.ParseUint(string(buf), 10, 32)
		fromBlock = uint32(from)
	}

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 32)
		if err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	subscribe()
	processQueue()
}

func subscribe() {
	_, err := jbClient.Subscribe(
		context.Background(),
		os.Getenv("ONESAT"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: func(txResp *jbModels.TransactionResponse) {
				log.Printf("[TX]: %d - %d: %s\n", txResp.BlockHeight, txResp.BlockIndex, txResp.Id)
				msgQueue <- &lib.Msg{
					Id:          txResp.Id,
					Height:      txResp.BlockHeight,
					Idx:         uint32(txResp.BlockIndex),
					Transaction: txResp.Transaction,
				}
			},
			// OnMempool:     onOneSatHandler,
			OnStatus: func(status *jbModels.ControlResponse) {
				log.Printf("[STATUS]: %v\n", status)
				msgQueue <- &lib.Msg{
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
	// go processInscriptionIds()
	go processTxns()
	for {
		msg := <-msgQueue

		switch msg.Status {
		case 0:
			tx, err := bt.NewTxFromBytes(msg.Transaction)
			if err != nil {
				log.Panicf("OnTransaction Parse Error: %s %+v\n", msg.Id, err)
			}

			txn := &lib.TxnStatus{
				ID:       msg.Id,
				Tx:       tx,
				Height:   msg.Height,
				Idx:      msg.Idx,
				Parents:  map[string]*lib.TxnStatus{},
				Children: map[string]*lib.TxnStatus{},
			}

			m.Lock()
			if t, ok := txns[msg.Id]; ok {
				t.Height = msg.Height
				t.Idx = msg.Idx
				m.Unlock()
				continue
			}
			for _, input := range tx.Inputs {
				if parent, ok := txns[input.PreviousTxIDStr()]; ok {
					parent.Children[msg.Id] = txn
					txn.Parents[parent.ID] = parent
				}
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
			err := os.WriteFile("last_block", []byte(fmt.Sprintf("%d", settledHeight)), 0644)
			if err != nil {
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
		go func(txn *lib.TxnStatus) {
			processTxn(txn)
			<-threadLimiter
		}(txn)
	}
}

func processTxn(txn *lib.TxnStatus) {
	fmt.Printf("Processing: %d %d %s\n", txn.Height, txn.Idx, txn.Tx.TxID())
	_, err := lib.Index1SatSpends(txn.Tx, true)
	if err != nil {
		log.Panic(err)
	}

	_, err = lib.IndexInscriptionTxos(txn.Tx, txn.Height, txn.Idx, true)
	if err != nil {
		log.Panic(err)
	}

	for _, child := range txn.Children {
		m.Lock()
		delete(child.Parents, txn.ID)
		orphan := len(child.Parents) == 0
		m.Unlock()
		if orphan {
			// inFlight++
			txnQueue <- child
		}
	}
	m.Lock()
	delete(txns, txn.ID)
	m.Unlock()
	wg.Done()
}
