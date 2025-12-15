package idx

import (
	"github.com/b-open-io/overlay/storage"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

// IndexedOutput is an alias to storage.IndexedOutput for backwards compatibility
type IndexedOutput = storage.IndexedOutput

// Owner is an alias to storage.Owner for backwards compatibility
type Owner = storage.Owner

// OwnerFromAddress creates an Owner from a Bitcoin address string
var OwnerFromAddress = storage.OwnerFromAddress

// OwnerFromBytes creates an Owner from raw 20 bytes
var OwnerFromBytes = storage.OwnerFromBytes

// Txo is the API response type that uses lib.Outpoint (underscore format)
type Txo struct {
	Outpoint *lib.Outpoint   `json:"outpoint"`
	Height   uint32          `json:"height,omitempty"`
	Idx      uint64          `json:"idx,omitempty"`
	Satoshis uint64          `json:"satoshis,omitempty"`
	Owners   []Owner         `json:"owners,omitempty"`
	Events   []string        `json:"events,omitempty"`
	Data     map[string]any  `json:"data,omitempty"`
	Spend    *chainhash.Hash `json:"spend,omitempty"`
}

// TxoFromIndexedOutput converts an IndexedOutput to a Txo for API responses
func TxoFromIndexedOutput(output *IndexedOutput) *Txo {
	if output == nil {
		return nil
	}
	return &Txo{
		Outpoint: lib.NewOutpoint(output.Outpoint),
		Height:   output.BlockHeight,
		Idx:      output.BlockIdx,
		Satoshis: output.Satoshis,
		Owners:   output.Owners,
		Events:   output.Events,
		Data:     output.Data,
		Spend:    output.SpendTxid,
	}
}

// TxosFromIndexedOutputs converts a slice of IndexedOutputs to Txos
func TxosFromIndexedOutputs(outputs []*IndexedOutput) []*Txo {
	txos := make([]*Txo, len(outputs))
	for i, output := range outputs {
		txos[i] = TxoFromIndexedOutput(output)
	}
	return txos
}

// OutpointToLib converts a transaction.Outpoint to lib.Outpoint
func OutpointToLib(op *transaction.Outpoint) *lib.Outpoint {
	if op == nil {
		return nil
	}
	return lib.NewOutpoint(*op)
}
