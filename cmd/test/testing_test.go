package test

import (
	"context"
	"encoding/json"
	"log"
	"testing"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/stretchr/testify/assert"
)

var hexId = "b70ec98aec25d49356092676cf46a452122b3c1b3b9bfc1e8e656c5d37a6f9af"
var ctx = context.Background()

func TestIngest(t *testing.T) {
	ingest := &idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: 1,
		Verbose:     true,
		Store:       config.Store,
	}

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
