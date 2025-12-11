package idx

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

func HeightScore(height uint32, idx uint64) float64 {
	if height == 0 {
		return float64(time.Now().UnixNano())
	}
	return float64(uint64(height)*1000000000 + idx)
}

type IndexContext struct {
	Tx       *transaction.Transaction `json:"-"`
	Txid     *chainhash.Hash          `json:"txid" swaggertype:"string"`
	TxidHex  string                   `json:"-"`
	Height   uint32                   `json:"height"`
	Idx      uint64                   `json:"idx"`
	Score    float64                  `json:"score"`
	Txos     []*Txo                   `json:"txos"`
	Spends   []*Txo                   `json:"spends"`
	Indexers []Indexer                `json:"-"`
	Ctx      context.Context          `json:"-"`
	Network  lib.Network              `json:"-"`
	Store    *QueueStore              `json:"-"`
	tags     []string                 `json:"-"`
}

func NewIndexContext(ctx context.Context, store *QueueStore, tx *transaction.Transaction, indexers []Indexer, network ...lib.Network) *IndexContext {
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
	return idxCtx.ParseTxos()
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
		spendCtx.ParseTxos()
		idxCtx.Spends = append(idxCtx.Spends, spendCtx.Txos[txin.SourceTxOutIndex])
	}
	return nil
}

func (idxCtx *IndexContext) ParseTxos() (err error) {
	accSats := uint64(0)
	for vout, txout := range idxCtx.Tx.Outputs {
		outpoint := lib.NewOutpointFromHash(idxCtx.Txid, uint32(vout))
		txo := &Txo{
			Outpoint: outpoint,
			Height:   idxCtx.Height,
			Idx:      idxCtx.Idx,
			Satoshis: &txout.Satoshis,
			OutAcc:   accSats,
			Data:     make(map[string]*IndexData),
		}
		if len(*txout.LockingScript) >= 25 && script.NewFromBytes((*txout.LockingScript)[:25]).IsP2PKH() {
			pkhash := lib.PKHash((*txout.LockingScript)[3:23])
			txo.AddOwner(pkhash.Address(idxCtx.Network))
		}
		idxCtx.Txos = append(idxCtx.Txos, txo)
		accSats += txout.Satoshis
		for _, indexer := range idxCtx.Indexers {
			if data := indexer.Parse(idxCtx, uint32(vout)); data != nil {
				txo.Data[indexer.Tag()] = data
			}
		}
	}
	for _, indexer := range idxCtx.Indexers {
		indexer.PreSave(idxCtx)
	}
	return nil
}

func (idxCtx *IndexContext) Save() error {
	if err := idxCtx.Store.SaveTxos(idxCtx); err != nil {
		log.Panic(err)
		return err
	} else if err := idxCtx.Store.SaveSpends(idxCtx); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}
