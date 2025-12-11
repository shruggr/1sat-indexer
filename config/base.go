package config

import (
	"context"

	"github.com/b-open-io/go-junglebus"
	"github.com/b-open-io/overlay/beef"
	"github.com/b-open-io/overlay/pubsub"
	"github.com/b-open-io/overlay/queue"
	"github.com/bsv-blockchain/arcade"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
	"github.com/shruggr/1sat-indexer/v5/mod/lock"
	"github.com/shruggr/1sat-indexer/v5/mod/onesat"
	"github.com/shruggr/1sat-indexer/v5/mod/p2pkh"
)

// Shared resources initialized by Config.Initialize()
var (
	// Store is the TXO storage backend
	Store *idx.QueueStore

	// QueueStorage is the underlying queue storage backend
	QueueStorage queue.QueueStorage

	// PubSub is the event publishing backend
	PubSub pubsub.PubSub

	// SSEManager manages SSE client subscriptions
	SSEManager *pubsub.SSEManager

	// Broadcaster is the transaction broadcaster
	Broadcaster *broadcaster.Arc

	// Network is the BSV network (mainnet/testnet)
	Network lib.Network

	// Chaintracks is the chain tracker for SPV validation
	Chaintracks arcade.Chaintracks

	// JungleBus is the JungleBus client
	JungleBus *junglebus.Client

	// BeefStorage is the BEEF storage chain
	BeefStorage *beef.Storage

	// Indexers is the list of active indexers
	Indexers []idx.Indexer

	// Ctx is the global context
	Ctx = context.Background()
)

// indexerRegistry maps indexer names to their constructors
var indexerRegistry = map[string]func() idx.Indexer{
	"p2pkh":       func() idx.Indexer { return &p2pkh.P2PKHIndexer{} },
	"lock":        func() idx.Indexer { return &lock.LockIndexer{} },
	"inscription": func() idx.Indexer { return &onesat.InscriptionIndexer{} },
	"ordlock":     func() idx.Indexer { return &onesat.OrdLockIndexer{} },
}

// CreateIndexers creates indexer instances from a list of names
func CreateIndexers(names []string) []idx.Indexer {
	indexers := make([]idx.Indexer, 0, len(names))
	for _, name := range names {
		if constructor, ok := indexerRegistry[name]; ok {
			indexers = append(indexers, constructor())
		}
	}
	return indexers
}

// RegisterIndexer allows registering custom indexers
func RegisterIndexer(name string, constructor func() idx.Indexer) {
	indexerRegistry[name] = constructor
}
