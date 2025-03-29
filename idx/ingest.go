package idx

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const PendingTxLog = "tx"
const RollbackTxLog = "rollback"
const ImmutableTxLog = "immutable"
const RejectedTxLog = "rejected"

type IngestCtx struct {
	Tag            string
	Key            string
	Indexers       []Indexer
	Concurrency    uint
	Verbose        bool
	PageSize       uint32
	Limit          uint32
	Network        lib.Network
	OnIngest       *func(ctx context.Context, idxCtx *IndexContext) error
	Once           bool
	Store          TxoStore
	AncestorConfig AncestorConfig
}

func (cfg *IngestCtx) IndexedTags() []string {
	var indexedTags = make([]string, 0, len(cfg.Indexers))
	for _, indexer := range cfg.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
	return indexedTags
}

func (cfg *IngestCtx) Exec(ctx context.Context) (err error) {
	limiter := make(chan struct{}, cfg.Concurrency)
	errors := make(chan error)
	done := make(chan string)
	inflight := make(map[string]struct{})
	ticker := time.NewTicker(15 * time.Second)
	ingestcount := 0
	txcount := uint32(0)
	// queueKey := QueueKey(cfg.Tag)
	statusTime := time.Now()
	wg := sync.WaitGroup{}
	lastScore := float64(0)
	for {
		select {
		case now := <-ticker.C:
			duration := time.Since(statusTime)
			log.Printf("Ingested %d in %ds - %.02ftx/s height %d", ingestcount, int(duration.Seconds()), float64(ingestcount)/duration.Seconds(), int(lastScore/1000000000))
			ingestcount = 0
			statusTime = now
		case txid := <-done:
			delete(inflight, txid)
			ingestcount++
		case err := <-errors:
			log.Println("Error", err)
			return err
		default:
			to := float64(time.Now().UnixNano())
			if logs, err := cfg.Store.Search(ctx, &SearchCfg{
				Keys:    []string{cfg.Key},
				Limit:   cfg.PageSize,
				Verbose: cfg.Verbose,
				To:      &to,
			}); err != nil {
				log.Panic(err)
			} else {
				if len(logs) == 0 {
					log.Println("No transactions to ingest")
					time.Sleep(time.Second)
				}
				for _, l := range logs {
					txid := l.Member
					lastScore = l.Score
					if _, ok := inflight[txid]; !ok {
						txcount++
						if cfg.Limit > 0 && txcount > cfg.Limit {
							wg.Wait()
							return nil
						}
						inflight[txid] = struct{}{}
						limiter <- struct{}{}
						wg.Add(1)
						go func(txid string) {
							defer func() {
								if r := recover(); r != nil {
									log.Panicln("Recovered in f", r)
								}
								<-limiter
								wg.Done()
								done <- txid
							}()
							if tx, err := jb.LoadTx(ctx, txid, true); err == jb.ErrNotFound {
								cfg.Store.Log(ctx, cfg.Key, txid, float64(time.Now().Add(15*time.Second).UnixNano()))
								return
							} else if err != nil {
								log.Panicf("LoadTx error %s %v", txid, err)
								errors <- err
							} else if idxCtx, err := cfg.IngestTx(ctx, tx, cfg.AncestorConfig); err != nil {
								log.Panicf("Ingest error %s %v", txid, err)
								errors <- err
							} else if cfg.OnIngest != nil {
								if err := (*cfg.OnIngest)(ctx, idxCtx); err != nil {
									errors <- err
								}
							} else if len(cfg.Key) > 0 {
								if err = cfg.Store.Delog(ctx, cfg.Key, txid); err != nil {
									log.Panic(err)
									return
								}
							}
						}(txid)
					}
				}
				// wg.Wait()
			}
		}
	}
}

func (cfg *IngestCtx) ParseTxid(ctx context.Context, txid string, ancestorCfg AncestorConfig) (*IndexContext, error) {
	if tx, err := jb.LoadTx(ctx, txid, true); err != nil {
		return nil, err
	} else {
		return cfg.ParseTx(ctx, tx, ancestorCfg)
	}
}

func (cfg *IngestCtx) ParseTx(ctx context.Context, tx *transaction.Transaction, ancestorCfg AncestorConfig) (idxCtx *IndexContext, err error) {
	idxCtx = NewIndexContext(ctx, cfg.Store, tx, cfg.Indexers, ancestorCfg, cfg.Network)
	err = idxCtx.ParseTxn()
	return
}

func (cfg *IngestCtx) IngestTxid(ctx context.Context, txid string, ancestorCfg AncestorConfig) (*IndexContext, error) {
	if cfg.Once {
		if score, err := cfg.Store.LogScore(ctx, LogKey(cfg.Tag), txid); err != nil {
			log.Panic(err)
			return nil, err
		} else if score > 0 {
			log.Println("[INGEST] Skipping", txid)
			return nil, nil
		}
	}
	if tx, err := jb.LoadTx(ctx, txid, true); err != nil {
		log.Println("LoadTx error", txid, err)
		return nil, err
	} else if tx == nil {
		return nil, fmt.Errorf("missing-txn %s", txid)
	} else {
		return cfg.IngestTx(ctx, tx, ancestorCfg)
	}
}

func (cfg *IngestCtx) IngestTx(ctx context.Context, tx *transaction.Transaction, ancestorCfg AncestorConfig) (idxCtx *IndexContext, err error) {
	start := time.Now()
	if idxCtx, err = cfg.ParseTx(ctx, tx, ancestorCfg); err != nil {
		return nil, err
	}

	err = cfg.Save(ctx, idxCtx)
	if err == nil && cfg.Verbose {
		log.Println("Ingested", idxCtx.TxidHex, idxCtx.Score/1000000000, time.Since(start))
	}
	return
}

func (cfg *IngestCtx) Save(ctx context.Context, idxCtx *IndexContext) (err error) {
	idxCtx.Save()
	if err = cfg.Store.Log(ctx, PendingTxLog, idxCtx.TxidHex, idxCtx.Score); err != nil {
		log.Panic(err)
		return
	} else if len(cfg.Tag) > 0 {
		if err = cfg.Store.Log(ctx, LogKey(cfg.Tag), idxCtx.TxidHex, idxCtx.Score); err != nil {
			log.Panic(err)
			return
		}
	}
	return
}

// func (cfg *IngestCtx) Rollback(ctx context.Context, txid string) error {
// 	if idxCtx, err := cfg.ParseTxid(ctx, txid, AncestorConfig{Load: true, Parse: true}); err != nil {
// 		return err
// 	} else {
// 		for _, spend := range idxCtx.Spends {
// 			if err = cfg.Store.RollbackSpend(ctx, spend, txid); err != nil {
// 				return err
// 			}
// 		}

// 		for _, txo := range idxCtx.Txos {
// 			if err = cfg.Store.RollbackTxo(ctx, txo); err != nil {
// 				return err
// 			}
// 		}

// 		// if err = cfg.Store.Log(ctx, RollbackTxLog, txid, float64(time.Now().UnixNano())); err != nil {
// 		// 	return err
// 		// } else if err = cfg.Store.Delog(ctx, PendingTxLog, txid); err != nil {
// 		// 	return err
// 		// }
// 	}
// 	return nil
// }
