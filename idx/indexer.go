package idx

// Indexer defines the interface for transaction output parsers.
// Implementations parse transaction outputs and return tag-specific data.
type Indexer interface {
	// Tag returns the unique identifier for this indexer
	Tag() string

	// Parse parses a transaction output and returns tag-specific data.
	// Returns nil if the output is not relevant to this indexer.
	// Events should be added directly to idxCtx.Outputs[vout].Events.
	Parse(idxCtx *IndexContext, vout uint32) any

	// PreSave is called before saving outputs, allowing batch processing
	PreSave(idxCtx *IndexContext)
}

// BaseIndexer provides default implementations for the Indexer interface
type BaseIndexer struct{}

func (b BaseIndexer) Tag() string {
	return ""
}

func (b BaseIndexer) Parse(idxCtx *IndexContext, vout uint32) any {
	return nil
}

func (b BaseIndexer) PreSave(idxCtx *IndexContext) {}
