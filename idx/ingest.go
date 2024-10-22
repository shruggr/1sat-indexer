package idx

import (
	"context"
	"log"
	"time"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/jb"
)

type IngestCtx struct {
	Tag         string
	Indexers    []Indexer
	Concurrency uint
	Verbose     bool
	limiter     chan struct{}
}

func (cfg *IngestCtx) Exec(ctx context.Context) (err error) {
	cfg.limiter = make(chan struct{}, cfg.Concurrency)
	errors := make(chan error)
	done := make(chan string)
	inflight := make(map[string]struct{})
	ticker := time.NewTicker(15 * time.Second)
	txcount := 0
	queueKey := QueueKey(cfg.Tag)
	for {
		select {
		case <-ticker.C:
			log.Println("Transactions - Ingested", txcount, txcount/15, "tx/s")
			txcount = 0
		case txid := <-done:
			delete(inflight, txid)
			txcount++
		case err := <-errors:
			log.Println("Error", err)
			return err
		default:
			if cfg.Verbose {
				log.Println("Loading", PAGE_SIZE, "transactions to ingest")
			}
			if txids, err := QueueDB.ZRangeArgs(ctx, redis.ZRangeArgs{
				Key:   queueKey,
				Start: 0,
				Stop:  PAGE_SIZE - 1,
			}).Result(); err != nil {
				log.Panic(err)
			} else {
				for _, txid := range txids {
					if _, ok := inflight[txid]; !ok {
						inflight[txid] = struct{}{}
						cfg.limiter <- struct{}{}
						go func(txid string) {
							defer func() {
								<-cfg.limiter
								done <- txid
							}()
							if _, err := cfg.IngestTxid(ctx, txid, AncestorConfig{
								Load:  true,
								Parse: true,
								Save:  true,
							}); err != nil {
								errors <- err
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
	idxCtx = NewIndexContext(ctx, tx, cfg.Indexers, ancestorCfg)
	idxCtx.ParseTxn()
	return
}

func (cfg *IngestCtx) IngestTxid(ctx context.Context, txid string, ancestorCfg AncestorConfig) (*IndexContext, error) {
	if tx, err := jb.LoadTx(ctx, txid, true); err != nil {
		return nil, err
	} else {
		return cfg.IngestTx(ctx, tx, ancestorCfg)
	}
}

func (cfg *IngestCtx) IngestTx(ctx context.Context, tx *transaction.Transaction, ancestorCfg AncestorConfig) (idxCtx *IndexContext, err error) {
	start := time.Now()
	if idxCtx, err = cfg.ParseTx(ctx, tx, ancestorCfg); err != nil {
		return nil, err
	}
	idxCtx.Save()
	queueKey := QueueKey(cfg.Tag)
	logKey := LogKey(cfg.Tag)
	if _, err := QueueDB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.ZAdd(idxCtx.Ctx, logKey, redis.Z{
			Score:  idxCtx.Score,
			Member: idxCtx.TxidHex,
		}).Err(); err != nil {
			log.Panic(err)
			return err
		} else if err := pipe.ZRem(ctx, queueKey, idxCtx.TxidHex).Err(); err != nil {
			return err
		}

		log.Println("Ingested", idxCtx.TxidHex, idxCtx.Score/1000000000, time.Since(start))
		return nil
	}); err != nil {
		log.Panic(err)
		return nil, err
	}
	return
}
