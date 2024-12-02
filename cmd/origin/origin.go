package main

import (
	"context"
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	redisstore "github.com/shruggr/1sat-indexer/v5/idx/redis-store"
	"github.com/shruggr/1sat-indexer/v5/mod/onesat"
)

var MAX_DEPTH uint
var CONCURRENCY uint
var ctx = context.Background()

// var originIndexer = &onesat.OriginIndexer{}
// var inscIndexer = &onesat.InscriptionIndexer{}

var ingest *idx.IngestCtx

var eventKey = evt.EventKey("origin", &evt.Event{
	Id:    "outpoint",
	Value: "",
})

var queue = make(chan string, 10000000)
var processed map[string]struct{}
var wg sync.WaitGroup

var store *redisstore.RedisStore

func init() {
	var err error
	if store, err = redisstore.NewRedisStore(os.Getenv("REDISTXO")); err != nil {
		panic(err)
	}
	ingest = &idx.IngestCtx{
		Tag:      "origin",
		Indexers: config.Indexers,
		Network:  config.Network,
		Store:    store,
	}
}

func main() {
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()
	go func() {
		limiter := make(chan struct{}, CONCURRENCY)
		for txid := range queue {
			if _, ok := processed[txid]; !ok {
				processed[txid] = struct{}{}
				limiter <- struct{}{}
				go func(txid string) {
					defer func() {
						<-limiter
						wg.Done()
					}()
					if err := ResolveOrigins(txid); err != nil {
						log.Panic(err)
					}
				}(txid)
			} else {
				wg.Done()
			}
		}
	}()

	for {
		searchCfg := &idx.SearchCfg{
			Key:   eventKey,
			Limit: 10000,
		}
		if outpoints, err := store.SearchOutpoints(ctx, searchCfg); err != nil {
			log.Panic(err)
		} else {
			processed = make(map[string]struct{}, 10000)
			txids := make(map[string]struct{}, len(outpoints))
			for _, outpoint := range outpoints {
				txids[outpoint[:64]] = struct{}{}
			}
			log.Println("Calculating origins for", len(txids), "txns")
			for txid := range txids {
				wg.Add(1)
				queue <- txid
			}
			wg.Wait()
			if len(outpoints) == 0 {
				time.Sleep(time.Second)
				log.Println("No results")
			}
		}
	}
}

func ResolveOrigins(txid string) (err error) {
	if idxCtx, err := ingest.IngestTxid(ctx, txid, idx.AncestorConfig{
		Load:  true,
		Parse: true,
		Save:  true,
	}); err != nil {
		log.Panic(err)
		return err
	} else {
		for _, spend := range idxCtx.Spends {
			if spend.Data[onesat.ORIGIN_TAG] != nil {
				origin := spend.Data[onesat.ORIGIN_TAG].Data.(*onesat.Origin)
				if origin.Outpoint == nil {
					log.Println("Queuing parent:", spend.Outpoint.String())
					wg.Add(1)
					queue <- spend.Outpoint.TxidHex()
				}
			}
		}
		resolved := make([]string, 0, len(idxCtx.Txos))
		for _, txo := range idxCtx.Txos {
			if txo.Data[onesat.ORIGIN_TAG] != nil {
				origin := txo.Data[onesat.ORIGIN_TAG].Data.(*onesat.Origin)
				if origin.Outpoint != nil {
					op := txo.Outpoint.String()
					resolved = append(resolved, op)
				}
			}
		}
		if len(resolved) > 0 {
			if err := store.Delog(ctx, eventKey, resolved...); err != nil {
				log.Panic(err)
			}
		}
	}
	return nil
}

// func ResolveOrigin(outpoint string, depth uint32) (origin *onesat.Origin, err error) {
// 	var txo *idx.Txo
// 	if txo, err = store.LoadTxo(ctx, outpoint, []string{onesat.ORIGIN_TAG}); err != nil {
// 		log.Panic(err)
// 		return nil, err
// 	} else if txo == nil {
// 		log.Panic("Missing txo:", outpoint)
// 	}
// 	origin = txo.Data[onesat.ORIGIN_TAG].Data.(*onesat.Origin)
// 	if origin.Outpoint == nil {

// 	}
// 	return origin, nil
// }
