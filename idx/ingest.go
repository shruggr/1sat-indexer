package idx

import (
	"context"
	"log"
	"time"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/jb"
)

type Ingest struct {
	Tag         string
	Indexers    []Indexer
	Concurrency uint
	Verbose     bool
}

func (cfg *Ingest) Exec(ctx context.Context) (err error) {
	limiter := make(chan struct{}, cfg.Concurrency)
	errors := make(chan error)
	done := make(chan *string)
	ticker := time.NewTicker(15 * time.Second)
	txcount := 0
	queueKey := IngestQueueKey
	for {
		select {
		case <-ticker.C:
			log.Println("Transactions - Ingested", txcount, txcount/15, "tx/s")
			txcount = 0
		case <-done:
			// log.Println("Processed", txid)
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
					// TODO: Add inflight check
					limiter <- struct{}{}
					go func(txid string) {
						defer func() {
							<-limiter
							done <- &txid

						}()
						if _, err := cfg.IngestTxid(ctx, txid); err != nil {
							errors <- err
						}
					}(txid)
				}
				if len(txids) == 0 {
					log.Println("No transactions to ingest")
					time.Sleep(time.Second)
				}
			}
		}
	}
}

func (cfg *Ingest) IngestTxid(ctx context.Context, txid string) (*IndexContext, error) {
	if tx, err := jb.LoadTx(ctx, txid, true); err != nil {
		return nil, err
	} else {
		return cfg.IngestTx(ctx, tx)
	}
}

func (cfg *Ingest) IngestTx(ctx context.Context, tx *transaction.Transaction) (idxCtx *IndexContext, err error) {
	start := time.Now()
	idxCtx = NewIndexContext(ctx, tx, cfg.Indexers, AncestorConfig{
		Load:  true,
		Parse: true,
		Save:  true,
	})
	idxCtx.ParseTxn()
	idxCtx.Save()
	if _, err := QueueDB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.ZAdd(idxCtx.Ctx, IngestLogKey, redis.Z{
			Score:  idxCtx.Score,
			Member: idxCtx.TxidHex,
		}).Err(); err != nil {
			log.Panic(err)
			return err
		} else if err := pipe.ZRem(ctx, IngestQueueKey, idxCtx.TxidHex).Err(); err != nil {
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
