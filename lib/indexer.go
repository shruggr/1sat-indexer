package lib

type Indexer interface {
	Tag() string
	Parse(idxCtx *IndexContext, vout uint32) (idxData *IndexData)
	PreSave(idxCtx *IndexContext)
	// PostSave(idxCtx *IndexContext)
	// Spend(idxCtx *IndexContext, vin uint32) (err error)
}

type BaseIndexer struct{}

func (b BaseIndexer) Tag() string {
	return ""
}

func (b BaseIndexer) Parse(idxCtx *IndexContext, vout uint32) (idxData *IndexData) {
	return
}

func (b BaseIndexer) PreSave(idxCtx *IndexContext) {}

// func (b BaseIndexer) PostSave(idxCtx *IndexContext) {
// 	return
// }

// func (b BaseIndexer) Spend(idxCtx *IndexContext, vin uint32) (err error) {
// 	return
// }
