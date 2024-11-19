package main

import (
	"context"
	"flag"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	redisstore "github.com/shruggr/1sat-indexer/v5/idx/redis-store"
)

var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string

func init() {
	flag.StringVar(&TAG, "tag", "ingest", "Ingest tag")
	flag.StringVar(&QUEUE, "q", "ingest", "Ingest tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()
}

func main() {
	ctx := context.Background()
	if err := (&idx.IngestCtx{
		Tag:         TAG,
		Key:         redisstore.QueueKey(QUEUE),
		Indexers:    config.Indexers,
		Network:     config.Network,
		Concurrency: CONCURRENCY,
		Verbose:     VERBOSE > 0,
	}).Exec(ctx); err != nil {
		panic(err)
	}
}
