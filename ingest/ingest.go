package ingest

import (
	"context"
	"log"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/lib"
)

func IngestTxid(ctx context.Context, txid string, indexers []lib.Indexer) (*lib.IndexContext, error) {
	if tx, err := lib.LoadTx(ctx, txid); err != nil {
		return nil, err
	} else {
		return IngestTx(ctx, tx, indexers)
	}
}

func IngestTx(ctx context.Context, tx *transaction.Transaction, indexers []lib.Indexer) (idxCtx *lib.IndexContext, err error) {
	idxCtx = lib.NewIndexContext(tx, indexers)
	idxCtx.ParseTxn(ctx)

	log.Printf("Ingesting %s", idxCtx.Txid)

	// ordinals.CalculateOrigins(idxCtx)
	// ordinals.ParseInscriptions(idxCtx)
	// lock.ParseLocks(idxCtx)
	// ordlock.ParseOrdinalLocks(idxCtx)
	idxCtx.Save(ctx)

	// We may want to check fees here to ensure the transaction will be mined
	// ordlock.ProcessSpends(ctx)

	// tickers := ordinals.IndexBsv20(idxCtx)

	// for _, tick := range tickers {
	// 	if len(tick) <= 16 {
	// 		lib.PublishEvent(context.Background(), "v1xfer", fmt.Sprintf("%x:%s", idxCtx.Txid, tick))
	// 	} else {
	// 		lib.PublishEvent(context.Background(), "v2xfer", fmt.Sprintf("%x:%s", idxCtx.Txid, tick))
	// 	}
	// }
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
