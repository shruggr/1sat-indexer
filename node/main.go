package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bitcoinsv/bsvd/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"
const TMP = "/tmp"

var THREADS uint64 = 64

var db *pgxpool.Pool
var bit *bitcoin.Bitcoind

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

	fmt.Println("BITCOIN", os.Getenv("BITCOIN_PORT"), os.Getenv("BITCOIN_HOST"))

	db, err = pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES"),
	)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
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
	go indexer.ProcessTxns(uint(THREADS))
	go processCompletions()

	var err error
	var block *bitcoin.BlockHeader

	row := db.QueryRow(context.Background(), `
		SELECT height
		FROM progress
		WHERE indexer = $1`,
		INDEXER,
	)
	var height uint32
	row.Scan(&height)
	if height < lib.TRIGGER {
		height = lib.TRIGGER
	}
	blk, err := bit.GetBlockByHeight(int(height))
	if err != nil {
		log.Panicln(err)
	}
	block, err = bit.GetBlockHeader(blk.Hash)
	if err != nil {
		log.Panicln(err)
	}

	for {
		if block == nil {
			blockHash, err := bit.GetBestBlockHash()
			if err != nil {
				log.Panicln(err)
			}
			block, err = bit.GetBlockHeader(blockHash)
			if err != nil {
				log.Panicln(err)
			}
		}
		if !isBlockIndexed(block.Hash) {
			for {
				if isBlockIndexed(block.PreviousBlockHash) || block.Height <= uint64(lib.TRIGGER) {
					break
				}
				fmt.Println("Crawling Back", block.Height-1, block.PreviousBlockHash)
				block, err = bit.GetBlockHeader(block.PreviousBlockHash)
				if err != nil {
					log.Panicln(err)
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

			_, err = io.Copy(f, r)
			if err != nil {
				log.Panicln(err)
			}
			f.Seek(0, 0)
			if err := processBlock(block, f); err != nil {
				log.Panicln(err)
			}

			if block.NextBlockHash != "" {
				fmt.Println("Crawling Forward", block.Height+1, block.NextBlockHash)
				block, err = bit.GetBlockHeader(block.NextBlockHash)
				if err != nil {
					log.Panicln(err)
				}
			} else {
				block = nil
			}
		} else {
			if block.NextBlockHash != "" {
				fmt.Println("Crawling Forward", block.Height+1, block.NextBlockHash)
				block, err = bit.GetBlockHeader(block.NextBlockHash)
				if err == nil {
					continue
				}
				log.Println("GetBlockHeader", err)
			} else {
				block = nil
			}
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
	height := uint32(block.Height)
	blockCtx := &indexer.BlockCtx{
		Hash:      block.Hash,
		Height:    &height,
		TxFees:    make([]*indexer.TxFee, int(txCount)),
		StartTime: time.Now(),
	}

	for idx = 0; idx < int(txCount); idx++ {
		txn := &indexer.TxnStatus{
			Height:   &height,
			Idx:      uint64(idx),
			Parents:  map[string]*indexer.TxnStatus{},
			Children: map[string]*indexer.TxnStatus{},
			Tx:       bt.NewTx(),
			Ctx:      blockCtx,
		}

		if _, err = txn.Tx.ReadFrom(f); err != nil {
			log.Panicln(block.Height, idx, err)
		}

		txid := txn.Tx.TxIDBytes()
		txn.ID = hex.EncodeToString(txid)

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

	_, err = db.Exec(context.Background(), `
		INSERT INTO blocks(id, height)
		VALUES(decode($1, 'hex'), $2)`,
		block.Hash,
		block.Height,
	)
	return err
}

func processCompletions() {
	for ctx := range settle {
		height := *ctx.Height - 6
		if _, err := db.Exec(context.Background(), `
			INSERT INTO progress(indexer, height)
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
		var wg sync.WaitGroup
		wg.Add(2)
		go func(height uint32) {
			fmt.Println("Processing inscription ids for height", height)
			err := lib.SetOriginNum(height)
			if err != nil {
				log.Panicln("Error processing inscription ids:", err)
			}
			wg.Done()
		}(height)

		go func(height uint32) {
			fmt.Println("Validating bsv20 for height", height)
			lib.ValidateBsv20(height)
			wg.Done()
		}(*ctx.Height)

		wg.Wait()
		// fmt.Println("Done processing inscription ids")
		rdb.Publish(context.Background(), "settled", fmt.Sprintf("%d", height))
	}
}

func isBlockIndexed(hash string) bool {
	rows, err := db.Query(context.Background(), `
		SELECT 1
		FROM blocks
		WHERE id=decode($1, 'hex')`,
		hash,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	return rows.Next()
}
