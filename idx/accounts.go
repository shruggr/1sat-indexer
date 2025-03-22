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
			log.Println("Syncing:", own)
			if lastHeight, err := ing.Store.LogScore(ctx, OwnerSyncKey, own); err != nil {
				return err
			} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
				log.Println("FetchOwnerTxns:", err)
				return err
			} else {
				var wg sync.WaitGroup
				for _, addTxn := range addTxns {
					if score, err := ing.Store.LogScore(ctx, tag, addTxn.Txid); err != nil {
						log.Panic(err)
						return err
					} else if score > 0 {
						continue
					}
					wg.Add(1)
					ing.Limiter <- struct{}{}
					go func(addTxn *jb.AddressTxn) {
						defer func() {
							<-ing.Limiter
							wg.Done()
						}()
						if _, err := ing.IngestTxid(ctx, addTxn.Txid, AncestorConfig{
							Load:  true,
							Parse: true,
							Save:  true,
						}); err != nil {
							log.Panic(err)
						}
					}(addTxn)

					if addTxn.Height > uint32(lastHeight) {
						lastHeight = float64(addTxn.Height)
					}
				}
				wg.Wait()
				if err := ing.Store.Log(ctx, OwnerSyncKey, own, lastHeight); err != nil {
					log.Panic(err)
				}
			}
		}
	}
	return nil
}
