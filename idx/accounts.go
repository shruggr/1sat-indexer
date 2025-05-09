package idx

import (
	"context"
	"log"
	"sync"

	"github.com/shruggr/1sat-indexer/v5/jb"
)

func SyncAcct(ctx context.Context, tag string, acct string, ing *IngestCtx) error {
	if owners, err := ing.Store.AcctOwners(ctx, acct); err != nil {
		return err
	} else {
		for _, own := range owners {
			SyncOwner(ctx, tag, own, ing)
		}
	}
	return nil
}

func SyncOwner(ctx context.Context, tag string, own string, ing *IngestCtx) error {
	log.Println("Syncing:", own)
	if lastHeight, err := ing.Store.LogScore(ctx, OwnerSyncKey, own); err != nil {
		return err
	} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
		log.Println("FetchOwnerTxns:", err)
		return err
	} else {
		limiter := make(chan struct{}, ing.Concurrency)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var firstErr error
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for _, addTxn := range addTxns {
			// Stop processing new iterations if the context is canceled
			if ctx.Err() != nil {
				break
			}

			if score, err := ing.Store.LogScore(ctx, tag, addTxn.Txid); err != nil {
				log.Panic(err)
				return err
			} else if score > 0 {
				continue
			}
			wg.Add(1)
			limiter <- struct{}{}
			go func(addTxn *jb.AddressTxn) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if _, err := ing.IngestTxid(ctx, addTxn.Txid, AncestorConfig{
					Load:  true,
					Parse: true,
					Save:  true,
				}); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel() // Cancel other goroutines and stop further iterations
					}
					mu.Unlock()
				}
			}(addTxn)

			if addTxn.Height > uint32(lastHeight) {
				lastHeight = float64(addTxn.Height)
			}
		}
		wg.Wait()

		if firstErr != nil {
			return firstErr
		}
		if err := ing.Store.Log(ctx, OwnerSyncKey, own, lastHeight); err != nil {
			return err
		}
	}
	return nil
}
