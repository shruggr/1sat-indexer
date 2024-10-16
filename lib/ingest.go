package lib

import (
	"context"

	"github.com/bitcoin-sv/go-sdk/transaction"
)

func IngestTxid(ctx context.Context, txid string, indexers []Indexer) (*IndexContext, error) {
	if tx, err := LoadTx(ctx, txid, true); err != nil {
		return nil, err
	} else {
		return IngestTx(ctx, tx, indexers)
	}
}

func IngestTx(ctx context.Context, tx *transaction.Transaction, indexers []Indexer) (idxCtx *IndexContext, err error) {
	idxCtx = NewIndexContext(ctx, tx, indexers, AncestorConfig{
		Load:  true,
		Parse: true,
		Save:  true,
	})
	idxCtx.ParseTxn()

	// log.Printf("Ingesting %d %d %s", idxCtx.Height, idxCtx.Idx, idxCtx.Txid)

	idxCtx.Save()
	return
}

// func IngestRawtx(ctx context.Context, rawtx []byte) (*lib.IndexContext, error) {
// 	if idxCtx, err := lib.ParseTxn(rawtx); err != nil {
// 		return nil, err
// 	} else if err = idxCtx.SaveSpends(ctx); err != nil {
// 		return nil, err
// 	} else {
// 		ordinals.CalculateOrigins(idxCtx)
// 		ordinals.ParseInscriptions(idxCtx)
// 		lock.ParseLocks(idxCtx)
// 		ordlock.ParseOrdinalLocks(idxCtx)
// 		idxCtx.Save()

// 		// We may want to check fees here to ensure the transaction will be mined
// 		// ordlock.ProcessSpends(ctx)

// 		tickers := ordinals.IndexBsv20(idxCtx)

// 		for _, tick := range tickers {
// 			if len(tick) <= 16 {
// 				lib.PublishEvent(context.Background(), "v1xfer", fmt.Sprintf("%x:%s", idxCtx.Txid, tick))
// 			} else {
// 				lib.PublishEvent(context.Background(), "v2xfer", fmt.Sprintf("%x:%s", idxCtx.Txid, tick))
// 			}
// 		}
// 		return idxCtx, nil
// 	}
// }
