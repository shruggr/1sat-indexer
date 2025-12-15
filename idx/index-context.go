package idx

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/b-open-io/overlay/storage"
	"github.com/bsv-blockchain/go-overlay-services/pkg/core/engine"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

// HeightScore calculates the score for a block height and index
func HeightScore(height uint32, idx uint64) float64 {
	if height == 0 {
		return float64(time.Now().UnixNano())
	}
	return float64(uint64(height)*1000000000 + idx)
}

// OutputContext holds additional context for an output during indexing
type OutputContext struct {
	OutAcc uint64 // Accumulated satoshis before this output
}

type IndexContext struct {
	Tx       *transaction.Transaction `json:"-"`
	Txid     *chainhash.Hash          `json:"txid" swaggertype:"string"`
	TxidHex  string                   `json:"-"`
	Height   uint32                   `json:"height"`
	Idx      uint64                   `json:"idx"`
	Score    float64                  `json:"score"`
	Outputs  []*IndexedOutput         `json:"outputs"`
	Spends   []*IndexedOutput         `json:"spends"`
	Indexers []Indexer                `json:"-"`
	Ctx      context.Context          `json:"-"`
	Network  lib.Network              `json:"-"`
	Store    *storage.OutputStore     `json:"-"`
	tags     []string                 `json:"-"`

	// OutputContexts holds additional per-output context not stored in IndexedOutput
	OutputContexts []*OutputContext `json:"-"`
	SpendContexts  []*OutputContext `json:"-"`
}

func NewIndexContext(ctx context.Context, store *storage.OutputStore, tx *transaction.Transaction, indexers []Indexer, network ...lib.Network) *IndexContext {
	if tx == nil {
		return nil
	}
	idxCtx := &IndexContext{
		Tx:       tx,
		Txid:     tx.TxID(),
		Indexers: indexers,
		Ctx:      ctx,
		Store:    store,
	}
	if len(network) > 0 {
		idxCtx.Network = network[0]
	} else {
		idxCtx.Network = lib.Mainnet
	}
	idxCtx.TxidHex = idxCtx.Txid.String()

	if tx.MerklePath != nil {
		idxCtx.Height = tx.MerklePath.BlockHeight
		for _, path := range tx.MerklePath.Path[0] {
			if idxCtx.Txid.IsEqual(path.Hash) {
				idxCtx.Idx = path.Offset
				break
			}
		}
	}
	idxCtx.Score = HeightScore(idxCtx.Height, idxCtx.Idx)
	for _, indexer := range indexers {
		idxCtx.tags = append(idxCtx.tags, indexer.Tag())
	}
	return idxCtx
}

func (idxCtx *IndexContext) ParseTxn() (err error) {
	if err = idxCtx.ParseSpends(); err != nil {
		return
	}
	return idxCtx.ParseOutputs()
}

func (idxCtx *IndexContext) ParseSpends() error {
	if idxCtx.Tx.IsCoinbase() {
		return nil
	}
	for _, txin := range idxCtx.Tx.Inputs {
		if txin.SourceTransaction == nil {
			return fmt.Errorf("missing source transaction for input %s", txin.SourceTXID)
		}
		// TODO: optimize to parse only the specific output being spent rather than full parent tx
		spendCtx := NewIndexContext(idxCtx.Ctx, nil, txin.SourceTransaction, idxCtx.Indexers, idxCtx.Network)
		spendCtx.ParseOutputs()
		idxCtx.Spends = append(idxCtx.Spends, spendCtx.Outputs[txin.SourceTxOutIndex])
		idxCtx.SpendContexts = append(idxCtx.SpendContexts, spendCtx.OutputContexts[txin.SourceTxOutIndex])
	}
	return nil
}

func (idxCtx *IndexContext) ParseOutputs() (err error) {
	accSats := uint64(0)
	for vout, txout := range idxCtx.Tx.Outputs {
		output := &IndexedOutput{
			Output: engine.Output{
				Outpoint: transaction.Outpoint{
					Txid:  *idxCtx.Txid,
					Index: uint32(vout),
				},
				BlockHeight: idxCtx.Height,
				BlockIdx:    idxCtx.Idx,
			},
			Satoshis: txout.Satoshis,
			Data:     make(map[string]interface{}),
		}
		outCtx := &OutputContext{
			OutAcc: accSats,
		}

		if len(*txout.LockingScript) >= 25 && script.NewFromBytes((*txout.LockingScript)[:25]).IsP2PKH() {
			pkhash := (*txout.LockingScript)[3:23]
			if err := output.AddOwnerFromBytes(pkhash); err == nil {
				// Owner added successfully
			}
		}

		idxCtx.Outputs = append(idxCtx.Outputs, output)
		idxCtx.OutputContexts = append(idxCtx.OutputContexts, outCtx)
		accSats += txout.Satoshis

		for _, indexer := range idxCtx.Indexers {
			if data := indexer.Parse(idxCtx, uint32(vout)); data != nil {
				output.SetData(indexer.Tag(), data)
			}
		}
	}
	for _, indexer := range idxCtx.Indexers {
		indexer.PreSave(idxCtx)
	}
	return nil
}

func (idxCtx *IndexContext) Save() error {
	if err := idxCtx.Store.SaveOutputs(idxCtx.Ctx, idxCtx.Outputs, idxCtx.TxidHex, idxCtx.Score); err != nil {
		log.Panic(err)
		return err
	}

	// Save spends
	for i, spend := range idxCtx.Spends {
		if spend != nil {
			input := idxCtx.Tx.Inputs[i]
			outpoint := fmt.Sprintf("%s.%d", input.SourceTXID.String(), input.SourceTxOutIndex)
			if err := idxCtx.Store.SaveSpend(idxCtx.Ctx, outpoint, idxCtx.TxidHex, idxCtx.Score); err != nil {
				log.Panic(err)
				return err
			}
		}
	}

	return nil
}

// GetOutAcc returns the accumulated satoshis before the output at the given index
func (idxCtx *IndexContext) GetOutAcc(vout uint32) uint64 {
	if int(vout) < len(idxCtx.OutputContexts) {
		return idxCtx.OutputContexts[vout].OutAcc
	}
	return 0
}

// GetSpendOutAcc returns the accumulated satoshis for a spend at the given index
func (idxCtx *IndexContext) GetSpendOutAcc(idx int) uint64 {
	if idx < len(idxCtx.SpendContexts) {
		return idxCtx.SpendContexts[idx].OutAcc
	}
	return 0
}
