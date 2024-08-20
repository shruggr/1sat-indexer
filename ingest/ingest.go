package ingest

import (
	"context"
	"fmt"

	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/lock"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

func IngestTxid(txid string) (*lib.IndexContext, error) {
	if rawtx, err := lib.LoadRawtx(txid); err != nil {
		return nil, err
	} else {
		return IngestRawtx(rawtx)
	}
}

func IngestRawtx(rawtx []byte) (*lib.IndexContext, error) {
	ctx, err := lib.ParseTxn(rawtx, "", 0, 0)
	if err != nil {
		return nil, err
	}
	ctx.SaveSpends()
	ordinals.CalculateOrigins(ctx)
	ordinals.ParseInscriptions(ctx)
	lock.ParseLocks(ctx)
	ordlock.ParseOrdinalLocks(ctx)
	ctx.Save()

	// We may want to check fees here to ensure the transaction will be mined
	// ordlock.ProcessSpends(ctx)

	tickers := ordinals.IndexBsv20(ctx)

	for _, tick := range tickers {
		if len(tick) <= 16 {
			lib.PublishEvent(context.Background(), "v1xfer", fmt.Sprintf("%x:%s", ctx.Txid, tick))
		} else {
			lib.PublishEvent(context.Background(), "v2xfer", fmt.Sprintf("%x:%s", ctx.Txid, tick))
		}
	}

	return ctx, nil
}
