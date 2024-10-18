package main

import (
	"context"
	"flag"
	"log"

	"github.com/shruggr/1sat-indexer/cmd/subscribe/sub"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var PORT int

var ctx = context.Background()
var TOPIC string
var FROM_BLOCK uint
var VERBOSE uint
var TAG string
var MEMOOOL bool
var BLOCK bool

func init() {
	flag.StringVar(&TAG, "tag", "", "Ingest tag")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	flag.UintVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&MEMOOOL, "m", false, "Index Mempool")
	flag.BoolVar(&BLOCK, "b", true, "Index Blocks")
	flag.Parse()
}

func main() {
	if err := (&sub.Sub{
		Tag:          TAG,
		Topic:        TOPIC,
		FromBlock:    FROM_BLOCK,
		IndexBlocks:  BLOCK,
		IndexMempool: MEMOOOL,
		Verbose:      VERBOSE > 0,
	}).Exec(ctx); err != nil {
		log.Panic(err)
	}
}