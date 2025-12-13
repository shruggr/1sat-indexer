package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/ingest"
)

var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string
var ROLLBACK bool

func init() {
	flag.StringVar(&TAG, "tag", idx.IngestTag, "Log tag")
	flag.StringVar(&QUEUE, "q", idx.IngestTag, "Queue tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&ROLLBACK, "r", false, "Enable rollback for old mempool transactions")
	flag.Parse()
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

	ingestCtx := &idx.IngestCtx{
		Tag:         TAG,
		Key:         idx.QueueKey(QUEUE),
		Indexers:    services.Indexers,
		Network:     services.Network,
		Concurrency: CONCURRENCY,
		Store:       services.Store,
		PageSize:    1000,
		Verbose:     VERBOSE > 0,
	}

	// Create the ingester with dependencies
	ingester := ingest.NewIngester(
		ingestCtx,
		services.Broadcaster,
		services.Chaintracks,
		services.BeefStorage,
		services.PubSub,
	)

	// Start the ingest service with queue processing, Arc callbacks, and audits
	ingester.Start(ctx, ROLLBACK)
}
