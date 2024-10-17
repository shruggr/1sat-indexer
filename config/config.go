package config

import (
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/lib"
)

var Indexers = []lib.Indexer{
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
