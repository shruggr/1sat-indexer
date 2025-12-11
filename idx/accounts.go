package idx

import (
	"context"
	"log"
	"sync"

	"github.com/shruggr/1sat-indexer/v5/jb"
)

func SyncAcct(ctx context.Context, tag string, acct string, ing *IngestCtx) error {
	log.Printf("SyncAcct: starting sync for account %s", acct)
	if owners, err := ing.Store.AcctOwners(ctx, acct); err != nil {
		log.Printf("SyncAcct: error getting owners for account %s: %v", acct, err)
		return err
	} else {
		log.Printf("SyncAcct: found %d owners for account %s", len(owners), acct)
		for i, own := range owners {
			log.Printf("SyncAcct: syncing owner %d/%d: %s", i+1, len(owners), own)
			if err := SyncOwner(ctx, tag, own, ing); err != nil {
				log.Printf("SyncAcct: error syncing owner %s: %v", own, err)
				return err
			}
		}
	}
	log.Printf("SyncAcct: completed sync for account %s", acct)
	return nil
}

func SyncOwner(ctx context.Context, tag string, own string, ing *IngestCtx) error {
	log.Printf("SyncOwner: starting sync for owner %s", own)
	if lastHeight, err := ing.Store.LogScore(ctx, OwnerSyncKey, own); err != nil {
		log.Printf("SyncOwner: error getting last height for %s: %v", own, err)
		return err
	} else {
		log.Printf("SyncOwner: last synced height for %s: %d", own, int(lastHeight))
		if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
			log.Printf("SyncOwner: FetchOwnerTxns error for %s: %v", own, err)
			return err
		} else {
			log.Printf("SyncOwner: processing %d transactions for %s", len(addTxns), own)
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
					log.Printf("SyncOwner: context canceled for %s, stopping", own)
					break
				}

				if score, err := ing.Store.LogScore(ctx, tag, addTxn.Txid); err != nil {
					log.Panic(err)
					return err
				} else if score > 0 {
					skipped++
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
						log.Printf("SyncOwner: error ingesting txid %s: %v", addTxn.Txid, err)
						mu.Lock()
						if firstErr == nil {
							firstErr = err
							cancel() // Cancel other goroutines and stop further iterations
						}
						mu.Unlock()
					} else {
						mu.Lock()
						processed++
						if processed%10 == 0 {
							log.Printf("SyncOwner: progress for %s: %d/%d transactions ingested", own, processed, len(addTxns)-skipped)
						}
						mu.Unlock()
					}
				}(addTxn)

				if addTxn.Height > uint32(lastHeight) {
					lastHeight = float64(addTxn.Height)
				}
			}

			log.Printf("SyncOwner: waiting for goroutines to complete for %s", own)
			wg.Wait()

			if firstErr != nil {
				log.Printf("SyncOwner: sync failed for %s with error: %v", own, firstErr)
				return firstErr
			}

			log.Printf("SyncOwner: successfully processed %d transactions (skipped %d) for %s", processed, skipped, own)
			if err := ing.Store.Log(ctx, OwnerSyncKey, own, lastHeight); err != nil {
				log.Printf("SyncOwner: error updating last height for %s: %v", own, err)
				return err
			}
			log.Printf("SyncOwner: updated last height to %d for %s", int(lastHeight), own)
		}
	}
	log.Printf("SyncOwner: completed sync for owner %s", own)
	return nil
}
