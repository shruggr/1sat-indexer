package test

// import (
// 	"context"
// 	"encoding/json"
// 	"log"

// 	"github.com/shruggr/1sat-indexer/config"
// 	"github.com/shruggr/1sat-indexer/idx"
// )

// var hexId = "ff423a984e59e762fc9d93198a5bee6ecf6ce1ba05c7385c833b8d4384be325d"

// func main() {
// 	ctx := context.Background()

// 	ingest := &idx.IngestCtx{
// 		Indexers:    config.Indexers,
// 		Concurrency: 1,
// 		Verbose:     true,
// 	}

// 	// beef := tx.

// 	if idxCtx, err := ingest.ParseTxid(ctx, hexId, idx.AncestorConfig{Load: true, Parse: true}); err != nil {
// 		log.Panic(err)
// 	} else {
// 		if out, err := json.MarshalIndent(idxCtx, "", "  "); err != nil {
// 			log.Panic(err)
// 		} else {
// 			log.Println(string(out))
// 		}
// 	}
// }
