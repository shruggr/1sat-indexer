package main

import (
	"context"
	"flag"
	"log"

	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/sub"
)

var POSTGRES string
var PORT int

var ctx = context.Background()
var TOPIC string
var FROM_BLOCK uint
var VERBOSE uint
var TAG string
var QUEUE string
var MEMOOOL bool
var BLOCK bool

func init() {
	flag.StringVar(&TAG, "tag", "", "(REQUIRED) Subscription Tag")
	flag.StringVar(&QUEUE, "q", "ingest", "Queue")
	flag.StringVar(&TOPIC, "t", "", "(REQUIRED) Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(bopen.TRIGGER), "Start from block")
	flag.UintVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&MEMOOOL, "m", false, "Index Mempool")
	flag.BoolVar(&BLOCK, "b", true, "Index Blocks")
	flag.Parse()

	if TAG == "" {
		log.Panic("Tag is required")
	}
	if TOPIC == "" {
		log.Panic("Topic is required")
	}
}

func main() {
	if err := (&sub.Sub{
		Tag:          TAG,
		Queue:        QUEUE,
		Topic:        TOPIC,
		FromBlock:    FROM_BLOCK,
		IndexBlocks:  BLOCK,
		IndexMempool: MEMOOOL,
		Verbose:      VERBOSE > 0,
	}).Exec(ctx); err != nil {
		log.Panic(err)
	}
}
