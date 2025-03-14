package config

import (
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

var Indexers = []idx.Indexer{}
var Broadcaster *broadcaster.Arc
var Network = lib.Mainnet
var Store idx.TxoStore
