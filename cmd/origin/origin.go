package main

import (
	"context"
	"log"
	"time"

	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
)

const MAX_DEPTH = 255

var PAGE_SIZE = uint32(1000)

// var CONCURRENCY = 1
var ctx = context.Background()
var originIndexer = &bopen.OriginIndexer{}

var ingest = &idx.IngestCtx{
	Tag:      "origin",
	Indexers: []idx.Indexer{originIndexer},
	PageSize: PAGE_SIZE,
	// Concurrency: uint(CONCURRENCY),
}

func main() {
	for {
		searchCfg := &idx.SearchCfg{
			Key: evt.EventKey("origin", &evt.Event{
				Id:    "outpoint",
				Value: "",
			}),
			Limit: PAGE_SIZE,
		}
		if outpoints, err := idx.Search(ctx, searchCfg); err != nil {
			log.Panic(err)
		} else {
			for _, outpoint := range outpoints {
				if op, err := lib.NewOutpointFromString(outpoint); err != nil {
					log.Panic(err)
				} else if origin, err := ResolveOrigin(op, 0); err != nil {
					log.Panic(err)
				} else if origin.Outpoint != nil {
					log.Println("Resolved origin:", outpoint, "->", origin.Outpoint.String(), origin.Nonce)
					if err := idx.TxoDB.ZRem(ctx, searchCfg.Key, outpoint).Err(); err != nil {
						log.Panic(err)
					}
				}
			}
			if len(outpoints) == 0 {
				// log.Println("No results")
				time.Sleep(time.Second)
			}
		}
	}
}

func ResolveOrigin(outpoint *lib.Outpoint, depth uint8) (origin *bopen.Origin, err error) {
	if idxCtx, err := ingest.ParseTxid(ctx, outpoint.TxidHex(), idx.AncestorConfig{
		Load:  true,
		Parse: true,
	}); err != nil {
		log.Panic(err)
	} else {
		txo := idxCtx.Txos[outpoint.Vout()]
		if originData, ok := txo.Data[bopen.ORIGIN_TAG]; ok {
			origin = originData.Data.(*bopen.Origin)
		}
		if origin.Outpoint == nil {
			satsIn := uint64(0)
			for _, spend := range idxCtx.Spends {
				if satsIn < txo.OutAcc {
					satsIn += *spend.Satoshis
					continue
				}
				if satsIn == txo.OutAcc && *spend.Satoshis == 1 {
					var parent *bopen.Origin
					if o, ok := spend.Data[bopen.ORIGIN_TAG]; ok {
						parent = o.Data.(*bopen.Origin)
					}

					if parent == nil || parent.Outpoint == nil {
						if depth >= MAX_DEPTH {
							log.Panicln("max depth reached")
						}
						if parent, err = ResolveOrigin(spend.Outpoint, depth+1); err != nil {
							log.Panic(err)
						}
					}
					origin.Outpoint = parent.Outpoint
					origin.Nonce = parent.Nonce + 1
				}
				break
			}
			if origin.Outpoint == nil {
				log.Panic("Failed to resolve origin:", outpoint.String())
			}
			log.Println("Resolved origin:", outpoint.String(), "->", origin.Outpoint.String(), origin.Nonce)
			if err := txo.SaveData(ctx); err != nil {
				log.Panic(err)
			}

		}
	}
	return origin, nil
}
