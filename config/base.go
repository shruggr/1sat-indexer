package config

import (
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
)

var Indexers = []idx.Indexer{}
var Broadcaster transaction.Broadcaster
var Network = lib.Mainnet
