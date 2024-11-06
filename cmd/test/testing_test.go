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

var hexId = "ecaa8afaf7ca76c5007bc7dd33d966fbb91bf51030c137bfafecc6bdd1bfac14"
var ctx = context.Background()

func TestIngest(t *testing.T) {
	ingest := &idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: 1,
		Verbose:     true,
	}

	idxCtx, err := ingest.IngestTxid(ctx, hexId, idx.AncestorConfig{Load: true, Parse: true, Save: true})
	assert.NoError(t, err)

	out, err := json.MarshalIndent(idxCtx, "", "  ")
	assert.NoError(t, err)
	log.Println(string(out))
}
