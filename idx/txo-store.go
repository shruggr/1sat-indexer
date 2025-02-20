package idx

import (
	"context"

	"github.com/shruggr/1sat-indexer/v5/lib"
)

type ComparisonType int

var (
	ComparisonOR  = ComparisonType(0)
	ComparisonAND = ComparisonType(1)
)

type SearchCfg struct {
	Keys           []string
	From           *float64
	To             *float64
	Limit          uint32
	ComparisonType ComparisonType
	ExcludeMined   bool
	ExcludeMempool bool
	Reverse        bool
	IncludeTxo     bool
	IncludeScript  bool
	IncludeTags    []string
	IncludeRawtx   bool
	OutpointsOnly  bool
	FilterSpent    bool
	RefreshSpends  bool
	Verbose        bool
}

type Log struct {
	Member string
	Score  float64
}

type TxoStore interface {
	AcctsByOwners(ctx context.Context, owners []string) ([]string, error)
	AcctOwners(ctx context.Context, acct string) ([]string, error)
	UpdateAccount(ctx context.Context, account string, owners []string) error
	LoadTxo(ctx context.Context, outpoint string, tags []string, script bool) (*Txo, error)
	LoadTxos(ctx context.Context, outpoints []string, tags []string, script bool) ([]*Txo, error)
	LoadTxosByTxid(ctx context.Context, txid string, tags []string, script bool) ([]*Txo, error)
	LoadData(ctx context.Context, outpoint string, tags []string) (IndexDataMap, error)
	SaveTxo(ctx context.Context, txo *Txo, height uint32, idx uint64) error
	RollbackTxo(ctx context.Context, txo *Txo) error
	SaveSpend(ctx context.Context, spend *Txo, txid string, height uint32, idx uint64) error
	RollbackSpend(ctx context.Context, spend *Txo, txid string) error
	GetSpend(ctx context.Context, outpoint string, refresh bool) (string, error)
	GetSpends(ctx context.Context, outpoints []string, refresh bool) ([]string, error)
	SetNewSpend(ctx context.Context, outpoint string, spend string) (bool, error)
	UnsetSpends(ctx context.Context, outpoints []string) error
	SaveTxoData(ctx context.Context, txo *Txo) error
	Search(ctx context.Context, cfg *SearchCfg) ([]*Log, error)
	SearchMembers(ctx context.Context, cfg *SearchCfg) ([]string, error)
	SearchOutpoints(ctx context.Context, cfg *SearchCfg) ([]string, error)
	SearchTxos(ctx context.Context, cfg *SearchCfg) ([]*Txo, error)
	SearchTxns(ctx context.Context, cfg *SearchCfg) ([]*lib.TxResult, error)
	SearchBalance(ctx context.Context, cfg *SearchCfg) (uint64, error)
	CountMembers(ctx context.Context, key string) (uint64, error)
	SyncAcct(ctx context.Context, tag, acct string, ingest *IngestCtx) error
	SyncOwner(ctx context.Context, tag, owner string, ingest *IngestCtx) error
	Log(ctx context.Context, key string, id string, score float64) error
	LogMany(ctx context.Context, key string, logs []Log) error
	LogOnce(ctx context.Context, key string, id string, score float64) (bool, error)
	Delog(ctx context.Context, key string, ids ...string) error
	LogScore(ctx context.Context, key string, id string) (float64, error)
}
