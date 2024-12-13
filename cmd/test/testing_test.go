package test

import (
	"context"
	"encoding/json"
	"log"
	"testing"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/broadcast"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/stretchr/testify/assert"
)

var hexId = "cb8d6af82c3b56c503b451c4136ca3ccf7beeb8eb10d08596333c2181e6afeb8"
var ctx = context.Background()

func TestIngest(t *testing.T) {
	ingest := &idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: 1,
		Verbose:     true,
		Store:       config.Store,
	}

	idxCtx, err := ingest.ParseTxid(ctx, hexId, idx.AncestorConfig{Load: true, Parse: true, Save: true})
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
