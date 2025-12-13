package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/mod/onesat"
	"github.com/shruggr/1sat-indexer/v5/sub"
)

var TOPIC string
var FROM_BLOCK uint
var VERBOSE uint
var TAG string
var QUEUE string
var MEMPOOL bool
var BLOCK bool
var REWIND bool

func init() {
	flag.StringVar(&TAG, "tag", "", "(REQUIRED) Subscription Tag")
	flag.StringVar(&QUEUE, "q", idx.IngestTag, "Queue")
	flag.StringVar(&TOPIC, "t", "", "(REQUIRED) Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(onesat.TRIGGER), "Start from block")
	flag.UintVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&MEMPOOL, "m", false, "Index Mempool")
	flag.BoolVar(&BLOCK, "b", true, "Index Blocks")
	flag.BoolVar(&REWIND, "r", false, "Reorg Rewind")
	flag.Parse()

	if TAG == "" {
		log.Panic("Tag is required")
	}
	if TOPIC == "" {
		log.Panic("Topic is required")
	}
}

func main() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(".env")

	ctx := context.Background()

	// Load and initialize services
	cfg, err := config.Load()
	if err != nil {
		log.Panic("Failed to load config:", err)
	}

	services, err := cfg.Initialize(ctx, nil)
	if err != nil {
		log.Panic("Failed to initialize services:", err)
	}

	if err := (&sub.Sub{
		Tag:          TAG,
		Queue:        QUEUE,
		Topic:        TOPIC,
		FromBlock:    FROM_BLOCK,
		IndexBlocks:  BLOCK,
		IndexMempool: MEMPOOL,
		Verbose:      VERBOSE > 0,
		ReorgRewind:  REWIND,
		Store:        services.Store,
		JungleBus:    services.JungleBus,
	}).Exec(ctx); err != nil {
		log.Panic(err)
	}
}
