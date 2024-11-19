package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	pgstore "github.com/shruggr/1sat-indexer/v5/idx/pg-store"
	"github.com/shruggr/1sat-indexer/v5/server"
)

var PORT int
var CONCURRENCY uint

var store *pgstore.PGStore

func init() {
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

	var err error
	if store, err = pgstore.NewPGStore(os.Getenv("POSTGRES_FULL")); err != nil {
		log.Panic(err)
	}
}

func main() {
	app := server.Initialize(&idx.IngestCtx{
		Tag:         "ingest",
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
		Once:        true,
		Store:       store,
	}, config.Broadcaster)
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}
