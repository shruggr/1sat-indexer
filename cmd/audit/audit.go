package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/v5/audit"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))
}

func main() {
	audit.StartTxAudit(context.Background(), &idx.IngestCtx{
		Indexers: config.Indexers,
		Network:  config.Network,
		Store:    config.Store,
	})
}
