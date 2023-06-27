package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bitcoinsv/bsvd/wire"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"

var THREADS uint64 = 16

var db *sql.DB

// var bit *bitcoin.Bitcoind

// var junglebusClient *junglebus.Client
// var msgQueue = make(chan *Msg, 1000000)

var height uint32 = 1

// var sub *junglebus.Subscription

var settle = make(chan *indexer.BlockCtx, 1000)

var rdb *redis.Client

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func init() {
	godotenv.Load("../.env")

	var err error
	// port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	// bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	// if err != nil {
	// 	log.Panic(err)
	// }

	// fmt.Println("YUGABYTE:", os.Getenv("YUGABYTE"))
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}
	db.SetConnMaxIdleTime(time.Millisecond * 100)
	db.SetMaxOpenConns(400)
	db.SetMaxIdleConns(25)

	fmt.Println("REDIS:", os.Getenv("REDIS"))
	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	// if os.Getenv("THREADS") != "" {
	// 	THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
	// 	if err != nil {
	// 		log.Panic(err)
	// 	}
	// }
}

func main() {
	go indexer.ProcessTxns(uint(THREADS))
	go processCompletions()

	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in f", r)
			log.Println("Exiting...")
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Printf("Caught signal")
		log.Println("Exiting...")
		os.Exit(0)
	}()

	var blockId string
	var height uint32
	for {
		func() {
			rows, err := db.Query(`SELECT encode(id, 'hex'), height
				FROM blocks 
				WHERE processed IS NULL
				ORDER BY height`,
			)
			failOnError(err, "Failed to query blocks")
			defer rows.Close()

			for rows.Next() {
				err = rows.Scan(&blockId, &height)
				failOnError(err, "Failed to scan block")
				if err = processBlock(height, blockId); err != nil {
					panic(err)
				}
			}
		}()
		time.Sleep(1 * time.Second)
	}
}

func processBlock(height uint32, blockId string) (err error) {
	fmt.Println("Processing Block", height)
	r, err := os.OpenFile(fmt.Sprintf("/home/shruggr/blocks/%s.bin", blockId), os.O_RDONLY, 0644)
	failOnError(err, "Failed to open block file")
	defer r.Close()

	// block, err := bit.GetBlockByHeight(int(height))
	// if err != nil {
	// 	log.Panicln(height, err)
	// }

	// // fmt.Printf("Block %s\n", block.Hash)
	// r, err := bit.GetRawBlockRest(block.Hash)
	// if err != nil {
	// 	log.Panicln(height, err)
	// }
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
		BlockId: blockId,
		Height:  height,
	}

	for idx = 0; idx < int(txCount); idx++ {

		txn := &lib.Txn{
			Tx:       bt.NewTx(),
			Height:   height,
			Idx:      uint64(idx),
			BlockId:  blockId,
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
	// rdb.Publish(context.Background(), "indexed", fmt.Sprintf("%d", indexer.Ctx.Height))
	settle <- indexer.Ctx
	return nil
}

func processCompletions() {
	for ctx := range settle {
		// fmt.Println("Processing Block Completion", ctx.Height, ctx.BlockId)
		_, err := db.Exec(`UPDATE blocks
			SET fees=$2, processed=current_timestamp
			WHERE id=decode($1, 'hex')`,
			ctx.BlockId,
			ctx.Fees,
		)
		if err != nil {
			log.Panic(err)
		}
		err = os.Remove(fmt.Sprintf("/home/shruggr/blocks/%s.bin", ctx.BlockId))
		failOnError(err, "Failed to remove block file")
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

		lib.ProcessBlockFees(settled)
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
