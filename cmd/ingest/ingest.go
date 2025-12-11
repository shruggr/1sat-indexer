package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/ingest"
)

var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string
var ROLLBACK bool
var ancestorConfig idx.AncestorConfig

func init() {
	flag.StringVar(&TAG, "tag", idx.IngestTag, "Log tag")
	flag.StringVar(&QUEUE, "q", idx.IngestTag, "Queue tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.BoolVar(&ROLLBACK, "r", false, "Enable rollback for old mempool transactions")
	flag.BoolVar(&ancestorConfig.Load, "l", false, "Load ancestors")
	flag.BoolVar(&ancestorConfig.Parse, "p", true, "Parse ancestors")
	flag.BoolVar(&ancestorConfig.Save, "s", true, "Save ancestors")
	flag.Parse()
	log.Println(ancestorConfig)
}

func main() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(".env")

	ctx := context.Background()

	// Setup Redis client for event publishing
	var redisClient *redis.Client
	if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
		log.Printf("Error parsing REDISEVT URL: %v", err)
	} else {
		redisClient = redis.NewClient(opts)
	}

	ingestCtx := &idx.IngestCtx{
		Tag:            TAG,
		Key:            idx.QueueKey(QUEUE),
		Indexers:       config.Indexers,
		Network:        config.Network,
		Concurrency:    CONCURRENCY,
		Store:          config.Store,
		PageSize:       1000,
		AncestorConfig: ancestorConfig,
		Verbose:        VERBOSE > 0,
	}

	// Start the ingest service with queue processing, Arc callbacks, and audits
	ingest.Start(ctx, ingestCtx, config.Broadcaster, redisClient, ROLLBACK)
}
