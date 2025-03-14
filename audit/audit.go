package audit

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/shruggr/1sat-indexer/v5/blk"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

var ctx = context.Background()
var headers = &blk.HeadersClient{Ctx: ctx}
var ingest *idx.IngestCtx
var mempoolScore = idx.HeightScore(50000000, 0)
var immutableScore float64
var arc *broadcaster.Arc

func StartTxAudit(ctx context.Context, ingestCtx *idx.IngestCtx, bcast *broadcaster.Arc, rollback bool) {
	ingest = ingestCtx
	arc = bcast
	blk.StartChaintipSub(ctx)

	for chaintip := range blk.C {
		log.Println("Chaintip", chaintip.Height, chaintip.Hash)
		immutableScore = idx.HeightScore(chaintip.Height-10, 0)
		AuditTransactions(ctx, rollback)
	}
}

func AuditTransactions(ctx context.Context, rollback bool) {
	cfg := &idx.SearchCfg{
		Keys: []string{idx.PendingTxLog},
	}

	// all pending transaction older than 2 minutes
	from := float64(-1 * time.Now().Add(-2*time.Minute).UnixNano())
	to := 0.0
	cfg.From = &from
	cfg.To = &to
	cfg.Limit = 100000
	limiter := make(chan struct{}, 20)
	var wg sync.WaitGroup
	if items, err := ingest.Store.Search(ctx, cfg); err != nil {
		log.Println(err)
	} else {
		log.Println("Audit pending txs", len(items))
		for _, item := range items {
			limiter <- struct{}{}
			wg.Add(1)
			go func(item *idx.Log) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if err := AuditTransaction(ctx, item.Member, item.Score, true); err != nil {
					log.Println("AuditTransaction:", err)
				}
			}(item)
		}
	}

	// Process mined txs
	from = 0.0
	cfg.From = &from
	cfg.To = &immutableScore
	if items, err := ingest.Store.Search(ctx, cfg); err != nil {
		log.Println("Search:", err)
	} else {
		log.Println("Audit mined txs", len(items))
		for _, item := range items {
			limiter <- struct{}{}
			wg.Add(1)
			go func(item *idx.Log) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if err := AuditTransaction(ctx, item.Member, item.Score, rollback); err != nil {
					log.Println("AuditTransaction:", err)
				}
			}(item)
		}
	}

	// Process mempool txs
	cfg.From = &mempoolScore
	until := time.Now().Add(-time.Hour)
	to = float64(until.UnixNano())
	cfg.To = &to
	if items, err := ingest.Store.Search(ctx, cfg); err != nil {
		log.Println("Search:", err)
	} else {
		log.Println("Audit mempool txs", len(items))
		for _, item := range items {
			limiter <- struct{}{}
			wg.Add(1)
			go func(item *idx.Log) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if err := AuditTransaction(ctx, item.Member, item.Score, rollback); err != nil {
					log.Println("AuditTransaction:", err)
				}
			}(item)
		}
	}
	wg.Wait()
}

func AuditTransaction(ctx context.Context, hexid string, score float64, rollback bool) error {
	// log.Println("Auditing", hexid)
	tx, err := jb.LoadTx(ctx, hexid, false)
	if err == jb.ErrNotFound {
		log.Println("Archive Missing", hexid)
		if err = ingest.Store.Rollback(ctx, hexid); err != nil {
			log.Println("Rollback error", hexid, err)
			return err
		} else if err := ingest.Store.Log(ctx, idx.RollbackTxLog, hexid, score); err != nil {
			log.Println("Delog error", hexid, err)
			return err
		}
	} else if err != nil {
		log.Println("LoadTx error", hexid, err)
		return err
	} else if tx.MerklePath == nil {
		log.Println("Fetching status for", hexid)
		if status, err := arc.Status(hexid); err != nil {
			return err
		} else if status.Status == 404 {
			if tx.MerklePath, err = jb.LoadProof(ctx, hexid); err != nil {
				log.Println("LoadProof error", hexid, err)
			}
		} else if status.Status == 200 || status.MerklePath != "" {
			// log.Println("MerklePath found", hexid)
			if tx.MerklePath, err = transaction.NewMerklePathFromHex(status.MerklePath); err != nil {
				log.Println("NewMerklePathFromHex error", hexid, err)
			}
		}
	}

	if tx.MerklePath == nil {
		if rollback && (score < 0 || (score > mempoolScore && score < float64(time.Now().Add(-2*time.Hour).UnixNano()))) {
			log.Println("Rollback", hexid)
			if err = ingest.Store.Rollback(ctx, hexid); err != nil {
				log.Println("Rollback error", hexid, err)
				return err
			} else if err := ingest.Store.Log(ctx, idx.RollbackTxLog, hexid, score); err != nil {
				log.Println("Delog error", hexid, err)
				return err
			}
		}
		return nil
	}

	txid := tx.TxID()
	var newScore float64
	if root, err := tx.MerklePath.ComputeRoot(txid); err != nil {
		log.Println("ComputeRoot error", hexid, err)
		return err
	} else if valid, err := headers.IsValidRootForHeight(root, tx.MerklePath.BlockHeight); err != nil {
		log.Println("IsValidRootForHeight error", hexid, err)
		return err
	} else if !valid {
		// TODO: Reload proof and revalidate
		log.Println("Invalid proof for", hexid)
		return nil
	}

	for _, path := range tx.MerklePath.Path[0] {
		if txid.IsEqual(path.Hash) {
			newScore = idx.HeightScore(tx.MerklePath.BlockHeight, path.Offset)
			break
		}
	}

	if newScore == 0 {
		// This should never happen
		log.Println("Transaction not in proof", hexid)
		return nil
	}

	if newScore != score {
		log.Println("Reingest", hexid, score, newScore)
		if _, err := ingest.IngestTx(ctx, tx, idx.AncestorConfig{
			Parse: true,
		}); err != nil {
			log.Println("IngestTx error", hexid, err)
			return err
		}
	}

	if newScore < immutableScore {
		log.Println("Archive Immutable", hexid, newScore)
		if err := ingest.Store.Log(ctx, idx.ImmutableTxLog, hexid, newScore); err != nil {
			log.Println("Log error", hexid, err)
			return err
		} else if err := ingest.Store.Delog(ctx, idx.PendingTxLog, hexid); err != nil {
			log.Println("Delog error", hexid, err)
			return err
		}
	}
	return nil
}
