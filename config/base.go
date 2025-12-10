package config

import (
	"context"
	"log"
	"os"

	"github.com/bsv-blockchain/arcade"
	"github.com/bsv-blockchain/arcade/chaintracks"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

var Indexers = []idx.Indexer{}
var Broadcaster *broadcaster.Arc
var Network = lib.Mainnet
var Store idx.TxoStore
var Chaintracks arcade.Chaintracks

// InitChaintracks initializes the chain tracker based on CHAINTRACKS_URL env var.
// If CHAINTRACKS_URL is set, uses the remote HTTP/SSE client.
// Otherwise, runs arcade locally with P2P.
func InitChaintracks(ctx context.Context) error {
	chaintracksURL := os.Getenv("CHAINTRACKS_URL")
	if chaintracksURL != "" {
		log.Printf("Using remote arcade server at %s", chaintracksURL)
		Chaintracks = chaintracks.NewClient(chaintracksURL)
		// Start SSE subscription so GetTip() has cached data
		Chaintracks.SubscribeTip(ctx)
		return nil
	}

	log.Println("Running arcade locally")
	network := os.Getenv("NETWORK")
	if network == "" {
		network = "main"
	}
	bootstrapURL := os.Getenv("BOOTSTRAP_URL")

	var err error
	arcadeInstance, err := arcade.NewArcade(ctx, arcade.Config{
		Network:      network,
		BootstrapURL: bootstrapURL,
	})
	if err != nil {
		return err
	}

	if err := arcadeInstance.Start(ctx); err != nil {
		return err
	}

	Chaintracks = arcadeInstance
	return nil
}
