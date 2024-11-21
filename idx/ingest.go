package idx

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const TxLogTag = "tx"

type IngestCtx struct {
	Tag         string
	Key         string
	Indexers    []Indexer
	Concurrency uint
	Verbose     bool
	PageSize    uint32
	Limit       uint32
	Network     lib.Network
	OnIngest    *func(ctx context.Context, idxCtx *IndexContext) error
	Once        bool
	Store       TxoStore
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
	wg := sync.WaitGroup{}
	for {
		select {
		case <-ticker.C:
			log.Println("Transactions - Ingested", ingestcount, ingestcount/15, "tx/s")
			ingestcount = 0
		case txid := <-done:
			delete(inflight, txid)
			ingestcount++
			wg.Done()
		case err := <-errors:
			log.Println("Error", err)
			return err
		default:
			if cfg.Verbose {
				log.Println("Loading", cfg.PageSize, "transactions to ingest")
			}
			if txids, err := cfg.Store.SearchMembers(ctx, &SearchCfg{
				Key:   cfg.Key,
				Limit: cfg.PageSize,
			}); err != nil {
				log.Panic(err)
			} else {
				for _, txid := range txids {
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
								<-limiter
								done <- txid
							}()
							if idxCtx, err := cfg.IngestTxid(ctx, txid, AncestorConfig{
								Load:  true,
								Parse: true,
							}); err != nil {
								errors <- err
							} else if cfg.OnIngest != nil {
								if err := (*cfg.OnIngest)(ctx, idxCtx); err != nil {
									errors <- err
								}
							}
						}(txid)
					}
				}
				if len(txids) == 0 {
					// log.Println("No transactions to ingest")
					time.Sleep(time.Second)
				}
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
	idxCtx.ParseTxn()
	return
}

func (cfg *IngestCtx) IngestTxid(ctx context.Context, txid string, ancestorCfg AncestorConfig) (*IndexContext, error) {
	if cfg.Once {
		if score, err := cfg.Store.LogScore(ctx, cfg.Tag, txid); err != nil {
			log.Panic(err)
			return nil, err
		} else if score > 0 {
			return nil, nil
		}
	}
	if tx, err := jb.LoadTx(ctx, txid, true); err != nil {
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
	if err = cfg.Store.Log(ctx, TxLogTag, idxCtx.TxidHex, idxCtx.Score); err != nil {
		log.Panic(err)
		return
	} else if len(cfg.Tag) > 0 {
		if err = cfg.Store.Log(ctx, LogKey(cfg.Tag), idxCtx.TxidHex, idxCtx.Score); err != nil {
			log.Panic(err)
			return
		} else if err = cfg.Store.Delog(ctx, QueueKey(cfg.Tag), idxCtx.TxidHex); err != nil {
			log.Panic(err)
			return
		}
	}
	return
}
