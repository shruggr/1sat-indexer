package test

import (
	"context"
	"encoding/json"
	"log"
	"testing"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/broadcast"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/stretchr/testify/assert"
)

var hexId = "8395796c2d9216cd2bdf4527bbc091e50b7967cc02cbf34e1d588d5fd8da9d4d"
var testCtx = context.Background()

// services holds the test services
var services *config.Services

// ingestCtx is created during test setup
var ingestCtx *idx.IngestCtx

func init() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	services, err = cfg.Initialize(testCtx, nil)
	if err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	ingestCtx = &idx.IngestCtx{
		Indexers:    services.Indexers,
		Concurrency: 1,
		Verbose:     true,
		Store:       services.Store,
	}
}

func TestIngest(t *testing.T) {
	idxResult, err := ingestCtx.IngestTxid(testCtx, hexId)
	assert.NoError(t, err)

	out, err := json.MarshalIndent(idxResult, "", "  ")
	assert.NoError(t, err)
	log.Println(string(out))
}

func TestArcStatus(t *testing.T) {
	status, err := services.Broadcaster.Status(hexId)
	assert.NoError(t, err)

	out, err := json.MarshalIndent(status, "", "  ")
	assert.NoError(t, err)
	log.Println(string(out))
}

func TestNoFeeTx(t *testing.T) {
	tx := transaction.NewTransaction()
	deps := &broadcast.BroadcastDeps{
		Store:       services.Store,
		JungleBus:   services.JungleBus,
		BeefStorage: services.BeefStorage,
		Arc:         services.Broadcaster,
	}
	resp := broadcast.Broadcast(testCtx, deps, tx)
	assert.Equal(t, int(resp.Status), fiber.StatusPaymentRequired)
}
