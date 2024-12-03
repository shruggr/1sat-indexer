package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/blk"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

const PAGE_SIZE = 10000

var JUNGLEBUS string
var ctx = context.Background()

var immutableScore float64
var headers = &blk.HeadersClient{Ctx: ctx}
var ingest *idx.IngestCtx

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

func main() {
	chaintip, err := blk.Chaintip(ctx)
	if err != nil {
		log.Panic(err)
	}
	immutableScore = idx.HeightScore(chaintip.Height-10, 0)
	// Initial from is 2 minutes ago

	from := float64(-1 * time.Now().Add(-2*time.Minute).UnixNano())
	cfg := &idx.SearchCfg{
		Key:   idx.PendingTxLog,
		From:  &from,
		Limit: 1000,
	}

	if items, err := config.Store.Search(ctx, cfg); err != nil {
		log.Panic(err)
	} else if len(items) == 0 {
		log.Println("No pending txs")
	} else {
		for _, item := range items {
			if err := AuditTransaction(item.Member, item.Score); err != nil {
				log.Panic(err)
			}
		}
	}
}

func AuditTransaction(hexid string, score float64) error {
	log.Println("Auditing", hexid)
	if tx, err := jb.LoadTx(ctx, hexid, true); err != nil {
		log.Panicln("LoadTx error", hexid, err)
	} else if tx == nil {
		// TODO: Handle missing tx
		log.Println("Missing tx", hexid)
	} else if tx.MerklePath == nil {
		// TODO: Handle no proof
		log.Println("No proof for", hexid)
	} else {
		txid := tx.TxID()
		var newScore float64
		if root, err := tx.MerklePath.ComputeRoot(txid); err != nil {
			log.Panicln("ComputeRoot error", hexid, err)
		} else if valid, err := headers.IsValidRootForHeight(root, tx.MerklePath.BlockHeight); err != nil {
			log.Panicln("IsValidRootForHeight error", hexid, err)
		} else if !valid {
			// TODO: Reload proof and revalidate
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
			if _, err := ingest.IngestTx(ctx, tx, idx.AncestorConfig{
				Load:  true,
				Parse: true,
			}); err != nil {
				log.Panicln("IngestTx error", hexid, err)
			}
		}
		if newScore < immutableScore {
			if err := config.Store.Log(ctx, idx.ImmutableTxLog, hexid, newScore); err != nil {
				log.Panicln("Log error", hexid, err)
			} else if err := config.Store.Delog(ctx, idx.PendingTxLog, hexid); err != nil {
				log.Panicln("Delog error", hexid, err)
			}
		}
	}
	return nil
}
