package config

import (
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/mod/lock"
	"github.com/shruggr/1sat-indexer/v5/mod/onesat"
	"github.com/shruggr/1sat-indexer/v5/mod/p2pkh"
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
