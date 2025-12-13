package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/server"
)

var CONCURRENCY uint
var VERBOSE int

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(".env")

	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()
}

func main() {
	ctx := context.Background()

	// Load configuration from config.yaml and environment
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logger
	logger := slog.Default()

	// Initialize all services
	services, err := cfg.Initialize(ctx, logger)
	if err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Create ingest context using the initialized services
	ingestCtx := &idx.IngestCtx{
		Tag:         idx.IngestTag,
		Indexers:    services.Indexers,
		Concurrency: CONCURRENCY,
		Network:     services.Network,
		Once:        true,
		Store:       services.Store,
		Verbose:     VERBOSE > 0,
		BeefStorage: services.BeefStorage,
	}

	// Initialize server with services for /chaintracks/ and /arcade/ routes
	app := server.Initialize(ingestCtx, services)

	log.Printf("Listening on :%d", cfg.Server.Port)
	if err := app.Listen(fmt.Sprintf(":%d", cfg.Server.Port)); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
