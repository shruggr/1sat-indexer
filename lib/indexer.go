package lib

type Indexer interface {
	Tag() string
	Parse(idxCtx *IndexContext, vout uint32) (idxData *IndexData, err error)
	PreSave(idxCtx *IndexContext) (err error)
	PostSave(idxCtx *IndexContext) (err error)
	Spend(idxCtx *IndexContext, vin uint32) (err error)
}

type BaseIndexer struct{}

func (b BaseIndexer) Tag() string {
	return ""
}
func (b BaseIndexer) Parse(idxCtx *IndexContext, vout uint32) (idxData *IndexData, err error) {
	return
}
func (b BaseIndexer) PreSave(idxCtx *IndexContext) (err error) {
	return
}
func (b BaseIndexer) PostSave(idxCtx *IndexContext) (err error) {
	return
}
func (b BaseIndexer) Spend(idxCtx *IndexContext, vin uint32) (err error) {
	return
}
