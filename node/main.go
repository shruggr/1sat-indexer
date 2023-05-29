package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"

var THREADS uint64 = 8

var db *sql.DB
var bit *bitcoin.Bitcoind

// var junglebusClient *junglebus.Client
// var msgQueue = make(chan *Msg, 1000000)

var height uint32

// var sub *junglebus.Subscription

var settle = make(chan *indexer.BlockCtx, 1000)

var rdb *redis.Client

func init() {
	godotenv.Load("../.env")

	var err error
	port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	if err != nil {
		log.Panic(err)
	}

	// fmt.Println("YUGABYTE:", os.Getenv("YUGABYTE"))
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}
	db.SetConnMaxIdleTime(time.Millisecond * 100)
	db.SetMaxOpenConns(400)
	db.SetMaxIdleConns(25)

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
	row := db.QueryRow(`SELECT height
		FROM progress
		WHERE indexer=$1`,
		INDEXER,
	)
	row.Scan(&height)
	fmt.Println("FromBlock", height)

	go indexer.ProcessTxns(uint(THREADS))
	go processCompletions()

	for {
		info, err := bit.GetInfo()
		if err != nil {
			panic(err)
		}
		fmt.Println("CurrentBlock", info.Blocks)
		for height < uint32(info.Blocks-6) {
			if err := processBlock(height); err != nil {
				panic(err)
			}
			height++
		}
		fmt.Println("Waiting for Block")
		time.Sleep(30 * time.Second)
	}
}

var bits4 = make([]byte, 4)
var bits32 = make([]byte, 32)

func processBlock(height uint32) (err error) {
	fmt.Println("Processing Block", height)
	block, err := bit.GetBlockByHeight(int(height))
	if err != nil {
		panic(err)
	}
	// fmt.Printf("Block %s\n", block.Hash)
	r, err := bit.GetRawBlockRest(block.Hash)
	if err != nil {
		return
	}
	// var n int
	// var n64 int64
	if _, err = io.ReadFull(r, bits4); err != nil {
		return
	}
	// fmt.Printf("version %d %d\n", bits4, n)
	if _, err = io.ReadFull(r, bits32); err != nil {
		return
	}
	// fmt.Printf("hash %x %d\n", bits32, n)
	if _, err = io.ReadFull(r, bits32); err != nil {
		return
	}
	// fmt.Printf("root %x %d\n", bits32, n)
	if _, err = io.ReadFull(r, bits4); err != nil {
		return
	}
	// fmt.Printf("time %d %d\n", bits4, n)
	if _, err = io.ReadFull(r, bits4); err != nil {
		return
	}
	// fmt.Printf("bits %d %d\n", bits4, n)
	if _, err = io.ReadFull(r, bits4); err != nil {
		return
	}
	// fmt.Printf("nonce %d %d\n", bits4, n)
	var txCount bt.VarInt
	if _, err = txCount.ReadFrom(r); err != nil {
		return
	}
	// fmt.Printf("txcount %d %d\n", txCount, n64)
	var idx int
	blockCtx := &indexer.BlockCtx{
		Height: height,
		TxFees: make([]*indexer.TxFee, int(txCount)),
	}

	for idx = 0; idx < int(txCount); idx++ {
		txn := &indexer.TxnStatus{
			Height:   height,
			Idx:      uint64(idx),
			Parents:  map[string]*indexer.TxnStatus{},
			Children: map[string]*indexer.TxnStatus{},
			Tx:       bt.NewTx(),
			Ctx:      blockCtx,
		}

		if _, err = txn.Tx.ReadFrom(r); err != nil {
			return
		}

		// fmt.Printf("Eval Txn %d\n", idx)
		txid := txn.Tx.TxIDBytes()
		txn.ID = hex.EncodeToString(txid)
		// indexer.M.Lock()
		// blockCtx.TxFees[idx] = &indexer.TxFee{Txid: txid}
		has1Sat := false
		for _, output := range txn.Tx.Outputs {
			if output.Satoshis == 1 {
				has1Sat = true
			}
		}
		if !has1Sat {
			continue
		}
		indexer.M.Lock()
		indexer.Txns[txn.ID] = txn
		for _, input := range txn.Tx.Inputs {
			inTxid := input.PreviousTxIDStr()
			if parent, ok := indexer.Txns[inTxid]; ok {
				parent.Children[txn.ID] = txn
				txn.Parents[parent.ID] = parent
			}
		}
		indexer.M.Unlock()

		if len(txn.Parents) == 0 {
			indexer.InQueue++
			indexer.Wg.Add(1)
			indexer.TxnQueue <- txn
		}
	}

	indexer.Wg.Wait()
	rdb.Publish(context.Background(), "indexed", fmt.Sprintf("%d", blockCtx.Height))
	settle <- blockCtx

	return nil
}

func processCompletions() {
	for ctx := range settle {
		// ctx.Wg.Wait()
		// var accFees uint64
		// for idx, txFee := range ctx.TxFees {
		// 	if err := lib.SaveTxn(txFee.Txid, height, uint64(idx), txFee.Fees, accFees); err != nil {
		// 		panic(err)
		// 	}
		// 	accFees += txFee.Fees
		// 	indexer.Inserts++
		// }
		if _, err := db.Exec(`INSERT INTO progress(indexer, height)
				VALUES($1, $2)
				ON CONFLICT(indexer) DO UPDATE
					SET height=$2`,
			INDEXER,
			height,
		); err != nil {
			log.Panic(err)
		}
		// indexer.Inserts++
		fmt.Printf("Completed: %d txns: %d\n", height, len(ctx.TxFees))
		fmt.Println("Processing inscription ids for height", height)
		err := lib.SetInscriptionIds(height)
		if err != nil {
			log.Panicln("Error processing inscription ids:", err)
		}
		lib.ValidateBsv20(height)
		rdb.Publish(context.Background(), "settled", fmt.Sprintf("%d", height))
	}
}
