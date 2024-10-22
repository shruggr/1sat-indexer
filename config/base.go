package config

import (
	"github.com/bitcoin-sv/go-sdk/transaction/broadcaster"
	"github.com/shruggr/1sat-indexer/idx"
)

var Indexers = []idx.Indexer{}
var Broadcaster *broadcaster.Arc
