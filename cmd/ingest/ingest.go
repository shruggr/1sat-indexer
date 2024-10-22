package main

import (
	"context"
	"flag"

	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
)

var CONCURRENCY uint
var VERBOSE int
var TAG string

func init() {
	flag.StringVar(&TAG, "tag", "ingest", "Ingest tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()
}

func main() {
	ctx := context.Background()
	if err := (&idx.IngestCtx{
		Tag:         TAG,
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Verbose:     VERBOSE > 0,
	}).Exec(ctx); err != nil {
		panic(err)
	}
}
