package main

import (
	"context"
	"flag"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/onesat"
)

var CONCURRENCY uint
var ctx = context.Background()

var originIndexer = &onesat.OriginIndexer{}
var inscIndexer = &onesat.InscriptionIndexer{}

var ingest = &idx.IngestCtx{
	Tag:      "origin",
	Indexers: []idx.Indexer{inscIndexer, originIndexer},
	Network:  config.Network,
	// Concurrency: uint(CONCURRENCY),
}

var eventKey = evt.EventKey("origin", &evt.Event{
	Id:    "outpoint",
	Value: "",
})

var queue = make(chan string, 1000000)
var resolved = make(chan string, 1000000)
var inflight = make(map[string]struct{})

func main() {
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()
	var wg sync.WaitGroup
	go func() {
		limiter := make(chan struct{}, CONCURRENCY)
		for {
			select {
			case outpoint := <-queue:
				if _, ok := inflight[outpoint]; !ok {
					inflight[outpoint] = struct{}{}
					wg.Add(1)
					limiter <- struct{}{}
					go func(outpoint string) {
						defer func() {
							<-limiter
						}()
						if _, err := ResolveOrigin(outpoint); err != nil {
							log.Panic(err)
						}
					}(outpoint)
				} else {
					// log.Println("Already in flight:", outpoint)
				}
			case outpoint := <-resolved:
				// log.Println("Resolved origin:", outpoint)
				delete(inflight, outpoint)
				if children, err := idx.TxoDB.ZRange(ctx, evt.EventKey("origin", &evt.Event{
					Id:    "parent",
					Value: outpoint,
				}), 0, -1).Result(); err != nil {
					log.Panic(err)
				} else {
					for _, child := range children {
						log.Println("Queuing child:", outpoint, "->", child)
						delete(inflight, child)
						queue <- child
					}
				}
				wg.Done()
			}
		}
	}()

	for {
		searchCfg := &idx.SearchCfg{
			Key:     eventKey,
			Reverse: true,
		}
		if outpoints, err := idx.SearchOutpoints(ctx, searchCfg); err != nil {
			log.Panic(err)
		} else {
			for _, outpoint := range outpoints {
				queue <- outpoint
			}
			time.Sleep(time.Second)
			if len(outpoints) == 0 {
				log.Println("No results")
				time.Sleep(time.Second)
			}
		}
	}
}

func ResolveOrigin(op string) (origin *onesat.Origin, err error) {
	if j, err := idx.TxoDB.HGet(ctx, idx.TxoDataKey(op), onesat.ORIGIN_TAG).Result(); err != nil && err != redis.Nil {
		log.Panic(op, err)
		return nil, err
	} else if err != redis.Nil {
		if origin, err = onesat.NewOriginFromBytes([]byte(j)); err != nil {
			log.Panic(err)
			return nil, err
		}
	}
	if origin == nil || origin.Outpoint == nil {
		if outpoint, err := lib.NewOutpointFromString(op); err != nil {
			log.Panic(err)
			return nil, err
		} else if idxCtx, err := ingest.ParseTxid(ctx, outpoint.TxidHex(), idx.AncestorConfig{
			Load:  true,
			Parse: true,
		}); err != nil {
			log.Panic(err)
			return nil, err
		} else {
			txo := idxCtx.Txos[outpoint.Vout()]
			log.Println("Saving Txo:", op)
			if err = txo.Save(ctx, idxCtx.Height, idxCtx.Idx); err != nil {
				log.Panic(err)
				return nil, err
			}
			origin = txo.Data[onesat.ORIGIN_TAG].Data.(*onesat.Origin)
			if origin.Outpoint == nil {
				if origin.Parent == nil {
					log.Panicln("missing-origin-parent", op)
				}
				log.Println("Queuing parent:", op, "->", origin.Parent.String())
				queue <- origin.Parent.String()
			}
		}
	}
	if origin.Outpoint != nil {
		log.Println("Resolved origin:", op, "->", origin.Outpoint.String(), origin.Nonce)
		if err := idx.TxoDB.ZRem(ctx, eventKey, op).Err(); err != nil {
			log.Panic(err)
		}
		resolved <- op
	} else {
		log.Println("Unresolved origin:", op)
	}
	return origin, nil
}
