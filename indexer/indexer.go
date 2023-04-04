package indexer

import (
	"fmt"
	"log"
	"sync"

	"github.com/libsv/go-bt/v2"
	"github.com/shruggr/1sat-indexer/lib"
)

var Txns = map[string]*TxnStatus{}
var TxnQueue = make(chan *TxnStatus, 1000000)
var M sync.Mutex
var Wg sync.WaitGroup
var InQueue uint32

type TxnStatus struct {
	ID       string
	Tx       *bt.Tx
	Height   uint32
	Idx      uint32
	Parents  map[string]*TxnStatus
	Children map[string]*TxnStatus
}

func ProcessTxns(THREADS uint) {
	threadLimiter := make(chan struct{}, THREADS)
	for {
		txn := <-TxnQueue
		threadLimiter <- struct{}{}
		go func(txn *TxnStatus) {
			processTxn(txn)
			<-threadLimiter
		}(txn)
	}
}

func processTxn(txn *TxnStatus) {
	// fmt.Printf("Processing: %d %d %s %d %d %v\n", txn.Height, txn.Idx, txn.Tx.TxID(), len(TxnQueue), len(Txns), InQueue)
	_, err := lib.IndexSpends(txn.Tx, true)
	if err != nil {
		log.Panic(err)
	}

	_, err = lib.IndexTxos(txn.Tx, txn.Height, txn.Idx, true)
	if err != nil {
		log.Panic(err)
	}

	if txn.Height > 0 {
		orphans := make([]*TxnStatus, 0)
		M.Lock()
		delete(Txns, txn.ID)
		for _, child := range txn.Children {
			delete(child.Parents, txn.ID)
			orphan := len(child.Parents) == 0
			if orphan {
				orphans = append(orphans, child)
			}
		}
		M.Unlock()
		for _, orphan := range orphans {
			Wg.Add(1)
			InQueue++
			TxnQueue <- orphan
		}
		fmt.Printf("Indexed: %d %d %s %d %d %v\n", txn.Height, txn.Idx, txn.Tx.TxID(), len(TxnQueue), len(Txns), InQueue)
		InQueue--
		Wg.Done()
	}
}

func ProcessInscriptionIds(settled chan uint32) {
	for {
		height := <-settled
		fmt.Println("Processing inscription ids for height", height)
		err := lib.SetInscriptionIds(height)
		if err != nil {
			log.Panicln("Error processing inscription ids:", err)
		}
	}
}
