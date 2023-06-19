package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bitcoinsv/bsvd/wire"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"

var THREADS uint64 = 16

var db *sql.DB
var bit *bitcoin.Bitcoind

// var junglebusClient *junglebus.Client
// var msgQueue = make(chan *Msg, 1000000)

var height uint32 = 1

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
		for height < uint32(info.Blocks)-6 {
			if err := processBlock(height); err != nil {
				panic(err)
			}
			height++
		}
	}
}

func processBlock(height uint32) (err error) {
	fmt.Println("Processing Block", height)
	block, err := bit.GetBlockByHeight(int(height))
	if err != nil {
		log.Panicln(height, err)
	}

	// fmt.Printf("Block %s\n", block.Hash)
	r, err := bit.GetRawBlockRest(block.Hash)
	if err != nil {
		log.Panicln(height, err)
	}
	protocolVersion := wire.ProtocolVersion
	wireBlockHeader := wire.BlockHeader{}
	err = wireBlockHeader.Bsvdecode(r, protocolVersion, wire.BaseEncoding)
	if err != nil {
		err = fmt.Errorf("ERROR: while opening block reader: %w", err)
		return
	}

	var txCount uint64
	txCount, err = wire.ReadVarInt(r, protocolVersion)
	if err != nil {
		err = fmt.Errorf("ERROR: while reading transaction count: %w", err)
		return
	}

	fmt.Printf("txcount %d\n", txCount)
	var idx int
	indexer.Ctx = &indexer.BlockCtx{
		BlockId: block.Hash,
		Height:  height,
	}

	for idx = 0; idx < int(txCount); idx++ {
		txn := &lib.Txn{
			Tx:       bt.NewTx(),
			Height:   height,
			Idx:      uint64(idx),
			BlockId:  block.Hash,
			Parents:  map[string]*lib.Txn{},
			Children: map[string]*lib.Txn{},
		}

		if _, err = txn.Tx.ReadFrom(r); err != nil {
			log.Panicln(height, idx, err)
		}

		txn.Id = txn.Tx.TxIDBytes()
		txn.HexId = hex.EncodeToString(txn.Id)

		indexer.M.Lock()
		indexer.Txns[txn.HexId] = txn
		for _, input := range txn.Tx.Inputs {
			inTxid := input.PreviousTxIDStr()
			if parent, ok := indexer.Txns[inTxid]; ok {
				parent.Children[txn.HexId] = txn
				txn.Parents[parent.HexId] = parent
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
	rdb.Publish(context.Background(), "indexed", fmt.Sprintf("%d", indexer.Ctx.Height))
	settle <- indexer.Ctx

	return nil
}

func processCompletions() {
	var subAcc uint64 = 0
	var subsidy uint64
	for ctx := range settle {
		if subAcc == 0 {
			row := db.QueryRow(`SELECT subacc FROM blocks
				WHERE height=$1`,
				ctx.Height-1,
			)
			row.Scan(&subAcc)
		}
		divisor := uint64(ctx.Height/210000 + 1)
		subsidy = uint64(5000000000) / divisor
		subAcc += subsidy
		_, err := db.Exec(`INSERT INTO blocks(id, height, subsidy, subacc, fees)
			VALUES(decode($1, 'hex'), $2, $3, $4, $5)
			ON CONFLICT DO NOTHING`,
			ctx.BlockId,
			ctx.Height,
			subsidy,
			subAcc,
			ctx.Fees,
		)
		if err != nil {
			log.Panic(err)
		}
		if height <= 6 {
			continue
		}
		settled := ctx.Height - 6
		if _, err = db.Exec(`INSERT INTO progress(indexer, height)
				VALUES($1, $2)
				ON CONFLICT(indexer) DO UPDATE
					SET height=$2
					WHERE progress.height < $2`,
			INDEXER,
			settled,
		); err != nil {
			log.Panic(err)
		}

		fmt.Printf("Completed: %d txns: %d\n", settled, ctx.Fees)
		fmt.Println("Processing inscription ids for height", settled)
		err = lib.SetOriginNums(settled)
		if err != nil {
			log.Panicln("Error processing inscription ids:", err)
		}
		lib.ValidateBsv20(settled)
		rdb.Publish(context.Background(), "settled", fmt.Sprintf("%d", settled))

	}
}
