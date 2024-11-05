package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/server"
)

var PORT int
var CONCURRENCY uint

var ingest *idx.IngestCtx

func init() {
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()
}

func main() {
	app := server.Initialize(&idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
	})
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}
