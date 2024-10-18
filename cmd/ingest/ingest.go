package main

import (
	"context"
	"flag"

	"github.com/shruggr/1sat-indexer/cmd/ingest/ingest"
	"github.com/shruggr/1sat-indexer/config"
)

var CONCURRENCY uint
var VERBOSE int
var TAG string

func init() {
	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()
}

func main() {
	ctx := context.Background()
	if err := (&ingest.Ingest{
		Tag:         TAG,
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Verbose:     VERBOSE > 0,
	}).Exec(ctx); err != nil {
		panic(err)
	}
}