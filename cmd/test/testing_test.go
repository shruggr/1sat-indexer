package test

import (
	"context"
	"encoding/json"
	"log"
	"testing"

	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/stretchr/testify/assert"
)

var hexId = "00d23de921071a14cedcd4301b89530d168833cf7fe0b04f03caaf099611f278"
var ctx = context.Background()

func TestIngest(t *testing.T) {
	ingest := &idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: 1,
		Verbose:     true,
	}

	idxCtx, err := ingest.IngestTxid(ctx, hexId, idx.AncestorConfig{Load: true, Parse: true})
	assert.NoError(t, err)

	out, err := json.MarshalIndent(idxCtx, "", "  ")
	assert.NoError(t, err)
	log.Println(string(out))
}
