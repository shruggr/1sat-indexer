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

var hexId = "39dc4858d9245b6b89f1e9008234d4328463393cf3a506f825a9c1332ebe2912"
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
