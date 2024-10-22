package config

import (
	"github.com/bitcoin-sv/go-sdk/transaction/broadcaster"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lock"
)

var Indexers = []idx.Indexer{
	&lock.LockIndexer{},
	&bopen.BOpenIndexer{},
	&bopen.InscriptionIndexer{},
	&bopen.MapIndexer{},
	&bopen.BIndexer{},
	&bopen.SigmaIndexer{},
	&bopen.OriginIndexer{},
	&bopen.Bsv21Indexer{},
	&bopen.Bsv20Indexer{},
	&bopen.OrdLockIndexer{},
}

// type MultiBroadcast int8
// var (
// 	MultiBroadcastAll MultiBroadcast = 0
// 	MultiBroadcastAny MultiBroadcast = 1
// )

// var Broadcasters = []transaction.Broadcaster{
// 	,
// }

var Broadcaster = &broadcaster.Arc{
	ApiUrl:        "https://arc.taal.com/v1",
	WaitForStatus: broadcaster.ACCEPTED_BY_NETWORK,
}
