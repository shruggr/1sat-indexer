package idx

import (
	"context"

	"github.com/shruggr/1sat-indexer/v5/lib"
)

type SearchCfg struct {
	Key           string
	From          float64
	To            float64
	Limit         uint32
	Reverse       bool
	IncludeTxo    bool
	IncludeScript bool
	IncludeTags   []string
	FilterSpent   bool
}

type SearchResult struct {
	Member string
	Score  float64
}

type TxoStore interface {
	AcctsByOwners(ctx context.Context, owners []string) ([]string, error)
	AcctOwners(ctx context.Context, acct string) ([]string, error)
	UpdateAccount(ctx context.Context, account string, owners []string) error
	LoadTxo(ctx context.Context, outpoint string, tags []string) (*Txo, error)
	LoadTxos(ctx context.Context, outpoints []string, tags []string) ([]*Txo, error)
	LoadData(ctx context.Context, outpoint string, tags []string) (IndexDataMap, error)
	SaveTxo(ctx context.Context, txo *Txo, height uint32, idx uint64) error
	SaveSpend(ctx context.Context, spend *Txo, txid string, height uint32, idx uint64) error
	GetSpend(ctx context.Context, outpoint string) (string, error)
	GetSpends(ctx context.Context, outpoints []string) ([]string, error)
	SetNewSpend(ctx context.Context, outpoint string, spend string) (bool, error)
	UnsetSpends(ctx context.Context, outpoints []string) error
	SaveTxoData(ctx context.Context, txo *Txo) error
	SearchMembers(ctx context.Context, cfg *SearchCfg) ([]string, error)
	SearchTxos(ctx context.Context, cfg *SearchCfg) ([]*Txo, error)
	SearchTxns(ctx context.Context, cfg *SearchCfg) ([]*lib.TxResult, error)
	CountMembers(ctx context.Context, key string) (uint64, error)
	SyncAcct(ctx context.Context, tag, acct string, ingest *IngestCtx) error
	SyncOwner(ctx context.Context, tag, owner string, ingest *IngestCtx) error
	Log(ctx context.Context, tag string, id string, score float64) error
	LogOnce(ctx context.Context, tag string, id string, score float64) (bool, error)
	Delog(ctx context.Context, tag string, ids ...string) error
	LogScore(ctx context.Context, tag string, id string) (float64, error)
}
