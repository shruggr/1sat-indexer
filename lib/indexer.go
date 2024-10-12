package lib

import "context"

type Indexer interface {
	Tag() string
	Parse(idxCtx *IndexContext, vout uint32) *IndexData
	PreSave(idxCtx *IndexContext)
	FromMap(data map[string]any) (obj any, err error)
	// UnmarshalSpend(data json.RawMessage) (obj any, err error)
	PostProcess(ctx context.Context, outpoint *Outpoint) error
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

func (b BaseIndexer) FromMap(data map[string]any) (obj any, err error) {
	return
}

func (b BaseIndexer) PostProcess(ctx context.Context, outpoint *Outpoint) error {
	return nil
}

// func (b BaseIndexer) UnmarshalSpend(data json.RawMessage) (any, error) {
// 	m := make(map[string]any)
// 	if err := json.Unmarshal(data, &m); err != nil {
// 		return nil, err
// 	}
// 	return m, nil
// }

// func (b BaseIndexer) PostSave(idxCtx *IndexContext) {
// 	return
// }

// func (b BaseIndexer) Spend(idxCtx *IndexContext, vin uint32) (err error) {
// 	return
// }
