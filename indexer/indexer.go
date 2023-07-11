package indexer

import (
	"bytes"
	"log"
	"sync"
	"time"

	"github.com/libsv/go-bt/v2"
	"github.com/shruggr/1sat-indexer/lib"
)

var Txns = map[string]*TxnStatus{}
var TxnQueue = make(chan *TxnStatus, 1000000)
var M sync.Mutex
var Wg sync.WaitGroup
var InQueue uint32

type TxFee struct {
	Txid []byte
	Fees uint64
}

type BlockCtx struct {
	Hash      string
	Height    *uint32
	TxFees    []*TxFee
	Wg        sync.WaitGroup
	TxCount   int
	StartTime time.Time
}

type TxnStatus struct {
	ID       string
	Tx       *bt.Tx
	Height   *uint32
	Idx      uint64
	Parents  map[string]*TxnStatus
	Children map[string]*TxnStatus
	Ctx      *BlockCtx
}

func ProcessTxns(THREADS uint) {
	threadLimiter := make(chan struct{}, THREADS)
	ticker := time.NewTicker(10 * time.Second)
	var txCount int
	var height uint32
	var idx uint64
	go func() {
		for range ticker.C {
			if txCount > 0 {
				log.Printf("Blk %d I %d - %d txs %d/s Q %d %d\n", height, idx, txCount, txCount/10, len(Txns), InQueue)
			}
			// m.Lock()
			txCount = 0
			// m.Unlock()
		}
	}()
	for {
		txn := <-TxnQueue
		threadLimiter <- struct{}{}
		go func(txn *TxnStatus) {
			processTxn(txn)
			txCount++
			if txn.Height != nil && *txn.Height > height {
				height = *txn.Height
				idx = txn.Idx
			} else if txn.Idx > idx {
				idx = txn.Idx
			}
			<-threadLimiter
		}(txn)
	}
}

func processTxn(txn *TxnStatus) {
	// fmt.Printf("Processing: %d %d %s %d %d %v\n", txn.Height, txn.Idx, txn.Tx.TxID(), len(TxnQueue), len(Txns), InQueue)
	blacklist := false
	for _, output := range txn.Tx.Outputs {
		if output.Satoshis == 1 && bytes.Contains(*output.LockingScript, []byte("Rekord IoT")) {
			blacklist = true
			break
		}
	}

	if !blacklist {
		_, err := lib.IndexTxn(txn.Tx, &txn.Ctx.Hash, txn.Height, txn.Idx, false)
		if err != nil {
			log.Panic(err)
		}
	}

	if txn.Height != nil {
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
			InQueue++
			Wg.Add(1)
			TxnQueue <- orphan
		}
		InQueue--
		Wg.Done()
	}
	// fmt.Printf("Indexed: %d %d %s %d %d %v\n", txn.Height, txn.Idx, txn.ID, len(TxnQueue), len(Txns), InQueue)
}
