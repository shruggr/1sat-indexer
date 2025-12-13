package idx

import (
	"context"
	"log"
	"sync"

	"github.com/b-open-io/go-junglebus"
)

func SyncOwner(ctx context.Context, tag string, own string, ing *IngestCtx, jb *junglebus.Client) error {
	lastHeight, err := ing.Store.LogScore(ctx, OwnerSyncKey, own)
	if err != nil {
		return err
	}

	addTxns, err := jb.GetAddressTransactions(ctx, own, uint32(lastHeight))
	if err != nil {
		return err
	}

	limiter := make(chan struct{}, ing.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var processed, skipped int
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, addTxn := range addTxns {
		// Stop processing new iterations if the context is canceled
		if ctx.Err() != nil {
			break
		}

		// Skip if already before last synced height (use < not <= to allow same-block txs)
		if float64(addTxn.BlockHeight) < lastHeight {
			skipped++
			continue
		}

		score, err := ing.Store.LogScore(ctx, tag, addTxn.TransactionID)
		if err != nil {
			log.Panic(err)
			return err
		}
		if score > 0 {
			skipped++
			continue
		}

		wg.Add(1)
		limiter <- struct{}{}
		go func(txid string, blockHeight uint32) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			if _, err := ing.IngestTxid(ctx, txid); err != nil {
				log.Printf("SyncOwner: error ingesting txid %s: %v", txid, err)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel() // Cancel other goroutines and stop further iterations
				}
				mu.Unlock()
			} else {
				mu.Lock()
				processed++
				mu.Unlock()
			}

			if float64(blockHeight) > lastHeight {
				mu.Lock()
				if float64(blockHeight) > lastHeight {
					lastHeight = float64(blockHeight)
				}
				mu.Unlock()
			}
		}(addTxn.TransactionID, addTxn.BlockHeight)
	}

	wg.Wait()

	if firstErr != nil {
		return firstErr
	}

	if err := ing.Store.Log(ctx, OwnerSyncKey, own, lastHeight); err != nil {
		return err
	}
	return nil
}
