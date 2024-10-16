package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/bitcoin-sv/go-sdk/chainhash"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/lib"
)

const PAGE_SIZE = 1000

var JUNGLEBUS string
var ctx = context.Background()

var CONCURRENCY int
var TAG string
var limiter chan struct{}

// var inflight map[string]struct{}

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.IntVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

	if err := lib.Initialize(); err != nil {
		log.Panic(err)
	}
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
	limiter = make(chan struct{}, CONCURRENCY)
}

var indexers = []lib.Indexer{
	&bopen.BOpenIndexer{},
	&bopen.InscriptionIndexer{},
	&bopen.MapIndexer{},
	&bopen.BIndexer{},
	&bopen.SigmaIndexer{},
	&bopen.OriginIndexer{},
	&bopen.Bsv21Indexer{},
	&bopen.Bsv20Indexer{},
	&bopen.OrdLockIndexer{},
}

var wg sync.WaitGroup
var done = make(chan *lib.IndexContext, 1000)

type TxQueueItem struct {
	txid     string
	score    float64
	parents  map[string]*TxQueueItem
	children map[string]*TxQueueItem
}

var txQueue = make(map[string]*TxQueueItem, 1000)

func main() {
	log.Println("CONCURRENCY:", CONCURRENCY)

	if err := lib.Queue.ZUnionStore(ctx, lib.IngestQueueKey, &redis.ZStore{
		Keys:      []string{lib.IngestQueueKey, lib.PendingQueueKey},
		Aggregate: "MIN",
	}).Err(); err != nil {
		log.Panic(err)
	} else if err := lib.Queue.Del(ctx, lib.PendingQueueKey).Err(); err != nil {
		log.Panic(err)
	}
	go func() {
		txcount := 0
		queueCount := 0
		ticker := time.NewTicker(15 * time.Second)
		for {
			select {
			case <-ticker.C:
				log.Println("Transactions - Ingested", txcount, txcount/15, "tx/s Queue", queueCount, queueCount/15, "tx/s")
				txcount = 0
			case idxCtx := <-done:
				txid := idxCtx.Txid.String()
				txItem, ok := txQueue[txid]
				if !ok {
					txItem = &TxQueueItem{
						txid:     txid,
						parents:  make(map[string]*TxQueueItem),
						children: make(map[string]*TxQueueItem),
					}
					txQueue[txid] = txItem
				}
				txItem.score = lib.HeightScore(idxCtx.Height, idxCtx.Idx)
				if len(idxCtx.Parents) > 0 {
					if _, err := lib.Queue.Pipelined(ctx, func(pipe redis.Pipeliner) error {
						for parent := range idxCtx.Parents {
							parentItem, ok := txQueue[parent]
							if !ok {
								parentItem = &TxQueueItem{
									txid:     parent,
									parents:  make(map[string]*TxQueueItem),
									children: make(map[string]*TxQueueItem, 1),
								}
								txQueue[parent] = parentItem
								if proof, _ := lib.LoadProof(ctx, parent); proof != nil {
									if parentTxid, err := chainhash.NewHashFromHex(parent); err == nil {
										for _, path := range proof.Path[0] {
											if parentTxid.IsEqual(path.Hash) {
												parentItem.score = lib.HeightScore(proof.BlockHeight, path.Offset)
												break
											}
										}
									}
								}

								// log.Println("Queueing parent", parent, "for", txid)
								if err := pipe.ZAddNX(ctx, lib.IngestQueueKey, redis.Z{
									Member: parent,
									Score:  parentItem.score,
								}).Err(); err != nil {
									return err
								}
							}
							parentItem.children[txid] = txItem
							txItem.parents[parent] = parentItem
						}
						if err := pipe.ZAdd(ctx, lib.PendingQueueKey, redis.Z{
							Member: txid,
							Score:  txItem.score,
						}).Err(); err != nil {
							return err
						} else if err := pipe.ZRem(ctx, lib.IngestQueueKey, txid).Err(); err != nil {
							return err
						}
						return nil
					}); err != nil {
						log.Panic(err)
					}
					queueCount++
				} else if _, err := lib.Queue.Pipelined(ctx, func(pipe redis.Pipeliner) error {
					for _, child := range txItem.children {
						delete(child.parents, txid)
						if len(child.parents) == 0 {
							if err := pipe.ZAdd(ctx, lib.IngestQueueKey, redis.Z{
								Member: child.txid,
								Score:  child.score,
							}).Err(); err != nil {
								return err
							} else if err := pipe.ZRem(ctx, lib.PendingQueueKey, child.txid).Err(); err != nil {
								return err
							}
						}
					}
					if err := pipe.ZRem(ctx, lib.IngestQueueKey, txid).Err(); err != nil {
						return err
					}
					delete(txQueue, txid)
					txcount++
					return nil
				}); err != nil {
					log.Panic(err)
				}
				<-limiter
				wg.Done()
			}
		}
	}()
	for {
		log.Println("Loading", PAGE_SIZE, "transactions to ingest")
		if txids, err := lib.Queue.ZRangeArgs(ctx, redis.ZRangeArgs{
			Key:   lib.IngestQueueKey,
			Start: 0,
			Stop:  PAGE_SIZE - 1,
		}).Result(); err != nil {
			log.Panic(err)
		} else {
			for _, txid := range txids {
				limiter <- struct{}{}
				wg.Add(1)
				go func(txid string) {
					if idxCtx, err := lib.IngestTxid(ctx, txid, indexers); err != nil {
						log.Panic(err)
					} else {
						done <- idxCtx
					}
				}(txid)
			}
			wg.Wait()
			if len(txids) == 0 {
				log.Println("No transactions to ingest")
				time.Sleep(time.Second)
			}
			// panic("done")
		}
	}
}
