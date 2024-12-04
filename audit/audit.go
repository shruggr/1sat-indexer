package audit

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/blk"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

var ctx = context.Background()
var headers = &blk.HeadersClient{Ctx: ctx}
var ingest *idx.IngestCtx
var immutableScore float64

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	ingest = &idx.IngestCtx{
		Indexers: config.Indexers,
		Network:  config.Network,
		Store:    config.Store,
	}
}

func StartTxAudit(ctx context.Context) {
	if tip, chaintips, err := blk.StartChaintipSub(ctx); err != nil {
		log.Panic(err)
	} else {
		chaintip := tip
		immutableScore = idx.HeightScore(chaintip.Height-10, 0)
		AuditTransactions(ctx)
		for chaintip = range chaintips {
			log.Println("Chaintip", chaintip.Height, chaintip.Hash)
			immutableScore = idx.HeightScore(chaintip.Height-10, 0)
			AuditTransactions(ctx)
		}
	}
}

func AuditTransactions(ctx context.Context) {
	cfg := &idx.SearchCfg{
		Key: idx.PendingTxLog,
	}

	// all pending transaction older than 2 minutes
	from := float64(-1 * time.Now().Add(-2*time.Minute).UnixNano())
	to := 0.0
	cfg.From = &from
	cfg.To = &to
	if items, err := config.Store.Search(ctx, cfg); err != nil {
		log.Panic(err)
	} else {
		log.Println("Audit pending txs", len(items))
		for _, item := range items {
			if err := AuditTransaction(ctx, item.Member, item.Score); err != nil {
				log.Panic(err)
			}
		}
	}

	// Process mined txs
	from = 0.0
	cfg.From = &from
	cfg.To = &immutableScore
	if items, err := config.Store.Search(ctx, cfg); err != nil {
		log.Panic(err)
	} else {
		log.Println("Audit mined txs", len(items))
		for _, item := range items {
			if err := AuditTransaction(ctx, item.Member, item.Score); err != nil {
				log.Panic(err)
			}
		}
	}

	// Process mempool txs
	from = 50000000.0
	cfg.From = &from
	until := time.Now().Add(-time.Hour)
	to = float64(until.UnixNano())
	cfg.To = &to
	if items, err := config.Store.Search(ctx, cfg); err != nil {
		log.Panic(err)
	} else {
		log.Println("Audit mempool txs", len(items))
		for _, item := range items {
			if err := AuditTransaction(ctx, item.Member, item.Score); err != nil {
				log.Panic(err)
			}
		}
	}
}

func AuditTransaction(ctx context.Context, hexid string, score float64) error {
	// log.Println("Auditing", hexid)
	tx, err := jb.LoadTx(ctx, hexid, true)
	if err != nil {
		log.Panicln("LoadTx error", hexid, err)
	} else if tx == nil {
		// TODO: Handle missing tx
		// Something bad has heppened if we get here
		log.Println("Missing tx", hexid)
		return nil
	}
	if tx.MerklePath == nil {
		log.Println("Fetching status for", hexid)
		if status, err := config.Broadcaster.Status(hexid); err != nil {
			return err
		} else if status.Status == 404 {
			log.Println("Status not found", hexid)
			// TODO: Handle missing tx
		} else if status.Status == 200 && status.MerklePath != "" {
			log.Println("MerklePath found", hexid)
			if tx.MerklePath, err = transaction.NewMerklePathFromHex(status.MerklePath); err != nil {
				log.Println("NewMerklePathFromHex error", hexid, err)
			}
		} else {
			log.Println("No proof", hexid)
			// TODO: Handle no proof

		}
	}
	if score < 0 {

	}
	if tx.MerklePath == nil {
		return nil
	}

	txid := tx.TxID()
	var newScore float64
	if root, err := tx.MerklePath.ComputeRoot(txid); err != nil {
		log.Panicln("ComputeRoot error", hexid, err)
	} else if valid, err := headers.IsValidRootForHeight(root, tx.MerklePath.BlockHeight); err != nil {
		log.Panicln("IsValidRootForHeight error", hexid, err)
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
		log.Panicln("Transaction not in proof", hexid)
	}

	if newScore != score {
		log.Println("Reingest", hexid, score, newScore)
		if _, err := ingest.IngestTx(ctx, tx, idx.AncestorConfig{
			Load:  true,
			Parse: true,
		}); err != nil {
			log.Panicln("IngestTx error", hexid, err)
		}
	}

	if newScore < immutableScore {
		log.Println("Archive Immutable", hexid, newScore)
		if err := config.Store.Log(ctx, idx.ImmutableTxLog, hexid, newScore); err != nil {
			log.Panicln("Log error", hexid, err)
		} else if err := config.Store.Delog(ctx, idx.PendingTxLog, hexid); err != nil {
			log.Panicln("Delog error", hexid, err)
		}
	}
	return nil
}
