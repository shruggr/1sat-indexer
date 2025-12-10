package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/server"
)

var PORT int
var CONCURRENCY uint
var VERBOSE int

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	PORT, _ = strconv.Atoi(os.Getenv("PORT"))
	flag.IntVar(&PORT, "p", PORT, "Port to listen on")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()
}

func main() {
	ctx := context.Background()
	if err := config.InitChaintracks(ctx); err != nil {
		log.Fatal("Failed to initialize chaintracks:", err)
	}
	jb.Chaintracks = config.Chaintracks

	app := server.Initialize(&idx.IngestCtx{
		Tag:         idx.IngestTag,
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
		Once:        true,
		Store:       config.Store,
		// Verbose:     VERBOSE > 0,
		Verbose: true,
	}, config.Broadcaster)
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}
