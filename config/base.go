package config

import (
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

var Indexers = []idx.Indexer{}
var Broadcaster transaction.Broadcaster
var Network = lib.Mainnet
var Store idx.TxoStore
