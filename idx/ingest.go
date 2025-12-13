package idx

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/b-open-io/overlay/beef"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const PendingTxLog = "tx"
const RollbackTxLog = "rollback"
const ImmutableTxLog = "immutable"
const RejectedTxLog = "rejected"

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
	Store       *QueueStore
	BeefStorage *beef.Storage
}

func (cfg *IngestCtx) IndexedTags() []string {
	var indexedTags = make([]string, 0, len(cfg.Indexers))
	for _, indexer := range cfg.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
	return indexedTags
}

func (cfg *IngestCtx) ParseTxid(ctx context.Context, txid string) (*IndexContext, error) {
	hash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, err
	}
	tx, err := cfg.BeefStorage.LoadTx(ctx, hash)
	if err != nil {
		return nil, err
	}
	return cfg.ParseTx(ctx, tx)
}

func (cfg *IngestCtx) ParseTx(ctx context.Context, tx *transaction.Transaction) (idxCtx *IndexContext, err error) {
	for _, input := range tx.Inputs {
		if input.SourceTransaction == nil {
			sourceTx, err := cfg.BeefStorage.LoadTx(ctx, input.SourceTXID)
			if err != nil {
				log.Println("LoadTx error", input.SourceTXID.String(), err)
				return nil, err
			}
			input.SourceTransaction = sourceTx
		}
	}
	idxCtx = NewIndexContext(ctx, cfg.Store, tx, cfg.Indexers, cfg.Network)
	err = idxCtx.ParseTxn()
	return
}

func (cfg *IngestCtx) IngestTxid(ctx context.Context, txid string) (*IndexContext, error) {
	if cfg.Once {
		if score, err := cfg.Store.LogScore(ctx, LogKey(cfg.Tag), txid); err != nil {
			log.Panic(err)
			return nil, err
		} else if score > 0 {
			log.Println("[INGEST] Skipping", txid)
			return nil, nil
		}
	}
	hash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, err
	}
	tx, err := cfg.BeefStorage.LoadTx(ctx, hash)
	if err != nil {
		log.Println("LoadTx error", txid, err)
		return nil, err
	}
	if tx == nil {
		return nil, fmt.Errorf("missing-txn %s", txid)
	}
	return cfg.IngestTx(ctx, tx)
}

func (cfg *IngestCtx) IngestTx(ctx context.Context, tx *transaction.Transaction) (idxCtx *IndexContext, err error) {
	start := time.Now()
	if idxCtx, err = cfg.ParseTx(ctx, tx); err != nil {
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
