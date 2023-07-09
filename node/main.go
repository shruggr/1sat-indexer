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

	"github.com/bitcoinsv/bsvd/wire"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"
const TMP = "/opt/tmp"

var THREADS uint64 = 128

var db *sql.DB
var bit *bitcoin.Bitcoind

// var junglebusClient *junglebus.Client
// var msgQueue = make(chan *Msg, 1000000)

// var height uint32

// var sub *junglebus.Subscription

var settle = make(chan *indexer.BlockCtx, 1000)

var rdb *redis.Client

// var indexed = map[string]bool{}

func init() {
	godotenv.Load("../.env")

	var err error
	port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	if err != nil {
		log.Panic(err)
	}

	fmt.Println("BITCOIN", os.Getenv("BITCOIN_PORT"), os.Getenv("BITCOIN_HOST"))

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

// var block *bitcoin.BlockHeader

func main() {
	// row := db.QueryRow(`SELECT height
	// 	FROM progress
	// 	WHERE indexer=$1`,
	// 	INDEXER,
	// )
	// row.Scan(&height)
	// fmt.Println("FromBlock", height)

	go indexer.ProcessTxns(uint(THREADS))
	go processCompletions()

	var err error
	var block *bitcoin.BlockHeader

	for {
		if block == nil {
			blockHash, err := bit.GetBestBlockHash()
			if err != nil {
				panic(err)
			}
			block, err = bit.GetBlockHeader(blockHash)
			if err != nil {
				panic(err)
			}
		}
		if !isBlockIndexed(block.Hash) {
			for {
				if isBlockIndexed(block.PreviousBlockHash) {
					break
				}
				fmt.Println("Crawling Back", block.Height-1, block.PreviousBlockHash)
				block, err = bit.GetBlockHeader(block.PreviousBlockHash)
				if err != nil {
					panic(err)
				}
			}
			fmt.Println("Processing Block", block.Height, block.Hash)

			// fmt.Printf("Block %s\n", block.Hash)
			r, err := bit.GetRawBlockRest(block.Hash)
			if err != nil {
				log.Panicln(err)
			}

			f, err := os.CreateTemp(TMP, block.Hash)
			if err != nil {
				log.Panicln(err)
			}

			fmt.Println("Downloading block", block.Height, block.Hash)
			_, err = io.Copy(f, r)
			if err != nil {
				log.Panicln(err)
			}
			f.Seek(0, 0)
			if err := processBlock(block, f); err != nil {
				panic(err)
			}

			if block.NextBlockHash != "" {
				block, err = bit.GetBlockHeader(block.NextBlockHash)
				if err != nil {
					panic(err)
				}
			} else {
				block = nil
			}
		} else {
			block = nil
			fmt.Println("Waiting for Block")
			time.Sleep(30 * time.Second)
		}
	}
}

func processBlock(block *bitcoin.BlockHeader, f *os.File) (err error) {
	defer func(f *os.File) {
		path := f.Name()
		err := f.Close()
		if err != nil {
			log.Println("Failed to close", path)
		}
		err = os.Remove(path)
		if err != nil {
			log.Println("Failed to remove", path)
		}
	}(f)
	protocolVersion := wire.ProtocolVersion
	wireBlockHeader := wire.BlockHeader{}
	err = wireBlockHeader.Bsvdecode(f, protocolVersion, wire.BaseEncoding)
	if err != nil {
		err = fmt.Errorf("ERROR: while opening block reader: %w", err)
		return
	}

	var txCount uint64
	txCount, err = wire.ReadVarInt(f, protocolVersion)
	if err != nil {
		err = fmt.Errorf("ERROR: while reading transaction count: %w", err)
		return
	}

	fmt.Printf("txcount %d\n", txCount)
	var idx int
	blockCtx := &indexer.BlockCtx{
		Hash:      block.Hash,
		Height:    uint32(block.Height),
		TxFees:    make([]*indexer.TxFee, int(txCount)),
		StartTime: time.Now(),
	}

	for idx = 0; idx < int(txCount); idx++ {
		txn := &indexer.TxnStatus{
			Height:   uint32(block.Height),
			Idx:      uint64(idx),
			Parents:  map[string]*indexer.TxnStatus{},
			Children: map[string]*indexer.TxnStatus{},
			Tx:       bt.NewTx(),
			Ctx:      blockCtx,
		}

		if _, err = txn.Tx.ReadFrom(f); err != nil {
			log.Panicln(block.Height, idx, err)
		}

		// fmt.Printf("Eval Txn %d\n", idx)
		txid := txn.Tx.TxIDBytes()
		txn.ID = hex.EncodeToString(txid)
		// indexer.M.Lock()
		// blockCtx.TxFees[idx] = &indexer.TxFee{Txid: txid}

		rdb.Set(context.Background(), txn.ID, txn.Tx.Bytes(), 0).Err()
		has1Sat := false
		for _, output := range txn.Tx.Outputs {
			if output.Satoshis == 1 {
				has1Sat = true
				break
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

	_, err = db.Exec(`INSERT INTO blocks(hash, height)
		VALUES(decode($1, 'hex'), $2)`,
		block.Hash,
		block.Height,
	)
	return err
}

func processCompletions() {
	for ctx := range settle {
		height := ctx.Height - 6
		if _, err := db.Exec(`INSERT INTO progress(indexer, height)
				VALUES($1, $2)
				ON CONFLICT(indexer) DO UPDATE
					SET height=$2
					WHERE progress.height < $2`,
			INDEXER,
			height,
		); err != nil {
			log.Panic(err)
		}

		fmt.Printf("Completed: %d txns: %d\n", height, len(ctx.TxFees))
		fmt.Println("Processing inscription ids for height", height)
		lib.ValidateBsv20(ctx.Height)

		err := lib.SetInscriptionIds(height)
		if err != nil {
			log.Panicln("Error processing inscription ids:", err)
		}
		rdb.Publish(context.Background(), "settled", fmt.Sprintf("%d", height))
	}
}

func isBlockIndexed(hash string) bool {
	rows, err := db.Query(`SELECT 1
		FROM blocks
		WHERE hash=decode($1, 'hex')`,
		hash,
	)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	return rows.Next()
}
