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
