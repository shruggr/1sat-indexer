package indexer

import (
	"sync"

	"github.com/shruggr/1sat-indexer/lib"
)

var Txns = map[string]*lib.Txn{}
var TxnQueue = make(chan *lib.Txn, 1000000)
var M sync.Mutex
var Wg sync.WaitGroup
var Ctx *BlockCtx

var InQueue uint32

// var queryCount uint64

//	type TxFee struct {
//		Txid []byte
//		Fees uint64
//	}
type BlockCtx struct {
	BlockId string
	Height  uint32
	Fees    uint64
	// Wg      sync.WaitGroup
}

// func ProcessTxns(THREADS uint) {
// 	threadLimiter := make(chan struct{}, THREADS)
// 	ticker := time.NewTicker(10 * time.Second)
// 	go func() {
// 		for t := range ticker.C {
// 			// select {
// 			// case t := <-ticker.C:
// 			log.Println(queryCount, "queries in 10 seconds", t.Format(time.RFC3339))
// 			queryCount = 0
// 			// }
// 		}
// 	}()
// 	for {
// 		txn := <-TxnQueue
// 		threadLimiter <- struct{}{}
// 		go func(txn *lib.Txn) {
// 			processTxn(txn)
// 			<-threadLimiter
// 		}(txn)
// 	}
// }

// func processTxn(txn *lib.Txn) {
// 	// fmt.Printf("Processing: %d %d %s %d %d %v\n", txn.Height, txn.Idx, txn.Tx.TxID(), len(TxnQueue), len(Txns), InQueue)
// 	result, err := txn.Index(false)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// 	queryCount += result.QueryCount
// 	Ctx.Fees += result.Fees

// 	if txn.Height > 0 {
// 		orphans := make([]*lib.Txn, 0)
// 		M.Lock()
// 		delete(Txns, txn.HexId)
// 		for _, child := range txn.Children {
// 			delete(child.Parents, txn.HexId)
// 			orphan := len(child.Parents) == 0
// 			if orphan {
// 				orphans = append(orphans, child)
// 			}
// 		}
// 		M.Unlock()
// 		for _, orphan := range orphans {
// 			InQueue++
// 			Wg.Add(1)
// 			TxnQueue <- orphan
// 		}
// 		InQueue--
// 		Wg.Done()
// 	}
// 	fmt.Printf("Indexed: %d %d %s %d %d %v\n", txn.Height, txn.Idx, txn.HexId, len(TxnQueue), len(Txns), InQueue)
// }
