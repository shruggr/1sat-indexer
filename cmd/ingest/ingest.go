package main

import (
	"context"
	"flag"
	"log"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string
var ancestorConfig idx.AncestorConfig

func init() {
	flag.StringVar(&TAG, "tag", idx.IngestTag, "Log tag")
	flag.StringVar(&QUEUE, "q", idx.IngestTag, "Queue tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&ancestorConfig.Load, "l", false, "Load ancestors")
	flag.BoolVar(&ancestorConfig.Parse, "p", true, "Parse ancestors")
	flag.BoolVar(&ancestorConfig.Save, "s", false, "Save ancestors")
	flag.Parse()
	log.Println(ancestorConfig)
}

func main() {
	ctx := context.Background()
	ingest := &idx.IngestCtx{
		Tag:            TAG,
		Key:            idx.QueueKey(QUEUE),
		Indexers:       config.Indexers,
		Network:        config.Network,
		Concurrency:    CONCURRENCY,
		Once:           true,
		Store:          config.Store,
		PageSize:       10000,
		AncestorConfig: ancestorConfig,
		Verbose:        true,
		// Verbose:     VERBOSE > 0,
	}

	if err := (ingest).Exec(ctx); err != nil {
		log.Println("Ingest error", err)
	}
}
