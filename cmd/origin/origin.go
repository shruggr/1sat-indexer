package main

import (
	"context"
	"log"
	"time"

	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/onesat"
)

const MAX_DEPTH = 255

var PAGE_SIZE = uint32(1000)

// var CONCURRENCY = 1
var ctx = context.Background()

// var originIndexer = &onesat.OriginIndexer{}

var ingest = &idx.IngestCtx{
	Tag:      "origin",
	Indexers: config.Indexers,
	PageSize: PAGE_SIZE,
	Network:  config.Network,
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
		if outpoints, err := idx.SearchOutpoints(ctx, searchCfg); err != nil {
			log.Panic(err)
		} else {
			for _, outpoint := range outpoints {
				if op, err := lib.NewOutpointFromString(outpoint); err != nil {
					log.Panic(err)
				} else if origin, err := ResolveOrigin(op, 0); err != nil {
					log.Panic(err)
				} else if origin.Outpoint != nil {
					log.Println("Origin:", outpoint, "->", origin.Outpoint.String(), origin.Nonce)
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

func ResolveOrigin(outpoint *lib.Outpoint, depth uint8) (origin *onesat.Origin, err error) {
	if idxCtx, err := ingest.ParseTxid(ctx, outpoint.TxidHex(), idx.AncestorConfig{
		Load:  true,
		Parse: true,
	}); err != nil {
		log.Panic(err)
		return nil, err
	} else {
		txo := idxCtx.Txos[outpoint.Vout()]
		originData, ok := txo.Data[onesat.ORIGIN_TAG]
		if ok {
			origin = originData.Data.(*onesat.Origin)
		}
		if origin.Outpoint == nil {
			satsIn := uint64(0)
			for _, spend := range idxCtx.Spends {
				if satsIn == txo.OutAcc && *spend.Satoshis == 1 && spend.Height >= onesat.TRIGGER {
					var parent *onesat.Origin
					if o, ok := spend.Data[onesat.ORIGIN_TAG]; ok {
						parent = o.Data.(*onesat.Origin)
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
					originData.Events = append(originData.Events, &evt.Event{
						Id:    "outpoint",
						Value: origin.Outpoint.String(),
					})
					break
				}
				satsIn += *spend.Satoshis
				if satsIn > txo.OutAcc {
					origin.Outpoint = txo.Outpoint
					break
				}

			}
			// if origin.Outpoint == nil {
			// 	origin.Outpoint = txo.Outpoint
			// }
			// if origin.Outpoint == nil {
			// 	log.Panic("Failed to resolve origin:", outpoint.String())
			// }
			// log.Println("Resolved origin:", outpoint.String(), "->", origin.Outpoint.String(), origin.Nonce)
		}
		if err := ingest.Save(ctx, idxCtx); err != nil {
			log.Panic(err)
		}
	}
	return origin, nil
}
