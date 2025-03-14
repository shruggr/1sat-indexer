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
var ctx = context.Background()
var ingest = &idx.IngestCtx{
	Indexers:    config.Indexers,
	Concurrency: 1,
	Verbose:     true,
	Store:       config.Store,
}

func TestIngest(t *testing.T) {
	idxCtx, err := ingest.IngestTxid(ctx, hexId, idx.AncestorConfig{Load: true, Parse: true, Save: true})
	assert.NoError(t, err)

	out, err := json.MarshalIndent(idxCtx, "", "  ")
	assert.NoError(t, err)
	log.Println(string(out))
}

func TestArcStatus(t *testing.T) {
	status, err := config.Broadcaster.Status(hexId)
	assert.NoError(t, err)

	out, err := json.MarshalIndent(status, "", "  ")
	assert.NoError(t, err)
	log.Println(string(out))
}

func TestNoFeeTx(t *testing.T) {
	tx := transaction.NewTransaction()
	resp := broadcast.Broadcast(context.Background(), config.Store, tx, config.Broadcaster)
	assert.Equal(t, int(resp.Status), fiber.StatusPaymentRequired)
}
