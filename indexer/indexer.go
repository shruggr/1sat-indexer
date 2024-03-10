package indexer

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

// var VERBOSE int
var CONCURRENCY int = 64
var threadLimiter chan struct{}

var Db *pgxpool.Pool
var Rdb *redis.Client
var junglebusClient *junglebus.Client

// var fromBlock uint

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint64
	Transaction []byte
}

func Initialize(db *pgxpool.Pool, rdb *redis.Client) (err error) {
	Db = db
	Rdb = rdb
	return lib.Initialize(db, rdb)
}

func Exec(
	indexBlocks bool,
	indexMempool bool,
	txHandler func(txn *lib.IndexContext) error,
	blockHander func(height uint32) error,
	indexer string,
	topic string,
	fromBlock uint,
	concurrency int,
	saveSpends bool,
	saveTxos bool,
	verbose int,
) (err error) {
	var wg sync.WaitGroup
	errors := make(chan error)

	threadLimiter = make(chan struct{}, concurrency)

	JUNGLEBUS := os.Getenv("JUNGLEBUS")
	if JUNGLEBUS == "" {
		JUNGLEBUS = "https://junglebus.gorillapool.io"
	}
	fmt.Println("JUNGLEBUS", JUNGLEBUS, topic)

	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

	if indexer != "" {
		var progress uint
		row := Db.QueryRow(context.Background(), `SELECT height
			FROM progress
			WHERE indexer=$1`,
			indexer,
		)
		err = row.Scan(&progress)
		if err != nil {
			Db.Exec(context.Background(),
				`INSERT INTO progress(indexer, height)
					VALUES($1, 0)`,
				indexer,
			)
		}
		progress -= 6
		if progress > fromBlock {
			fromBlock = progress
		}
	}

	var txCount int
	var height uint32
	var idx uint64
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			if txCount > 0 {
				log.Printf("Blk %d I %d - %d txs %d/s\n", height, idx, txCount, txCount/10)
			}
			txCount = 0
		}
	}()

	var sub *junglebus.Subscription

	eventHandler := junglebus.EventHandler{
		OnStatus: func(status *models.ControlResponse) {
			if verbose > 0 {
				log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
			}
			if status.StatusCode == 200 {
				wg.Wait()
				err = blockHander(status.Block)
				height = status.Block
				// var settledHeight uint32
				// if status.Block > 6 {
				// 	settledHeight = status.Block - 6
				// } else {
				// 	settledHeight = 0
				// }

				if indexer != "" {
					if _, err := Db.Exec(context.Background(),
						`UPDATE progress
						SET height=$2
						WHERE indexer=$1 and height<$2`,
						indexer,
						height,
					); err != nil {
						log.Panic(err)
					}
				}
				fromBlock = uint(status.Block) + 1
			}
			if status.StatusCode == 999 {
				log.Println(status.Message)
				log.Println("Unsubscribing...")
				sub.Unsubscribe()
				os.Exit(0)
				return
			}
		},
		OnError: func(err error) {
			log.Printf("[ERROR]: %v\n", err)
			errors <- err
		},
	}

	if indexBlocks {
		eventHandler.OnTransaction = func(txn *models.TransactionResponse) {
			if verbose > 0 {
				log.Printf("[TX]: %d - %d: %d %s\n", txn.BlockHeight, txn.BlockIndex, len(txn.Transaction), txn.Id)
			}
			threadLimiter <- struct{}{}
			wg.Add(1)
			Rdb.Set(context.Background(), txn.Id, txn.Transaction, 0).Err()
			txCount++
			height = txn.BlockHeight
			idx = txn.BlockIndex
			go func(txn *models.TransactionResponse) {
				defer func() {
					<-threadLimiter
					wg.Done()
				}()
				txCtx, err := lib.ParseTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
				if err != nil {
					log.Panicln(txn.Id, err)
				}
				if saveSpends {
					txCtx.SaveSpends()
				}
				if txHandler != nil {
					err = txHandler(txCtx)
					if err != nil {
						log.Panicln(err)
					}
				}
				if saveTxos {
					txCtx.Save()
				}
				if indexer != "" {
					Db.Exec(context.Background(),
						`INSERT INTO txn_indexer(txid, indexer) 
						VALUES ($1, $2)
						ON CONFLICT DO NOTHING`,
						txCtx.Txid,
						indexer,
					)
				}
			}(txn)
		}
	}
	if indexMempool {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			if verbose > 0 {
				log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			}
			threadLimiter <- struct{}{}
			Rdb.Set(context.Background(), txn.Id, txn.Transaction, 0).Err()
			txCount++
			go func(txn *models.TransactionResponse) {
				defer func() {
					<-threadLimiter
				}()
				txCtx, err := lib.ParseTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
				if err != nil {
					log.Panic(err)
				}
				if saveSpends {
					txCtx.SaveSpends()
				}
				if txHandler != nil {
					err = txHandler(txCtx)
					if err != nil {
						log.Panic(err)
					}
				}
				if saveTxos {
					txCtx.Save()
				}
			}(txn)
		}
	}

	log.Println("Subscribing to Junglebus from block", fromBlock)
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		topic,
		uint64(fromBlock),
		eventHandler,
	)
	if err != nil {
		return err
	}
	defer func() {
		sub.Unsubscribe()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Printf("Caught signal")
		fmt.Println("Unsubscribing and exiting...")
		sub.Unsubscribe()
		os.Exit(0)
	}()

	err = <-errors
	return err
}
