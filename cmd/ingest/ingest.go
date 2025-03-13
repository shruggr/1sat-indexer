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
	flag.BoolVar(&ancestorConfig.Parse, "p", false, "Parse ancestors")
	flag.BoolVar(&ancestorConfig.Parse, "s", false, "Save ancestors")
	flag.Parse()

}

func main() {
	ctx := context.Background()
	ingest := &idx.IngestCtx{
		Tag:         TAG,
		Key:         idx.QueueKey(QUEUE),
		Indexers:    config.Indexers,
		Network:     config.Network,
		Concurrency: CONCURRENCY,
		Once:        true,
		// Verbose:     VERBOSE > 0,
		Verbose:        true,
		Store:          config.Store,
		PageSize:       10000,
		AncestorConfig: ancestorConfig,
	}

	if err := (ingest).Exec(ctx); err != nil {
		log.Println("Ingest error", err)
	}
}
