package main

import (
	"context"
	"flag"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string

func init() {
	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.StringVar(&QUEUE, "q", "ingest", "Ingest tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()
}

func main() {
	ctx := context.Background()
	if err := (&idx.IngestCtx{
		Tag:         TAG,
		Key:         idx.QueueKey(QUEUE),
		Indexers:    config.Indexers,
		Network:     config.Network,
		Concurrency: CONCURRENCY,
		Verbose:     VERBOSE > 0,
		Store:       config.Store,
	}).Exec(ctx); err != nil {
		panic(err)
	}
}
