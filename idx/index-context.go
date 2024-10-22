package idx

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/bitcoin-sv/go-sdk/chainhash"
	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/jb"
	"github.com/shruggr/1sat-indexer/lib"
	// "github.com/shruggr/1sat-indexer/data"
)

type AncestorConfig struct {
	Load  bool
	Parse bool
	Save  bool
}

type IndexContext struct {
	Tx             *transaction.Transaction `json:"-"`
	Txid           *chainhash.Hash          `json:"txid"`
	TxidHex        string                   `json:"-"`
	Height         uint32                   `json:"height"`
	Idx            uint64                   `json:"idx"`
	Score          float64                  `json:"score"`
	Txos           []*Txo                   `json:"txos"`
	Spends         []*Txo                   `json:"spends"`
	Indexers       []Indexer                `json:"-"`
	Ctx            context.Context          `json:"-"`
	tags           []string                 `json:"-"`
	ancestorConfig AncestorConfig           `json:"-"`
}

func NewIndexContext(ctx context.Context, tx *transaction.Transaction, indexers []Indexer, ancestorConfig AncestorConfig) *IndexContext {
	idxCtx := &IndexContext{
		Tx:             tx,
		Txid:           tx.TxID(),
		Indexers:       indexers,
		Ctx:            ctx,
		ancestorConfig: ancestorConfig,
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
	} else {
		idxCtx.Height = uint32(time.Now().Unix())
	}
	idxCtx.Score = HeightScore(idxCtx.Height, idxCtx.Idx)
	for _, indexer := range indexers {
		idxCtx.tags = append(idxCtx.tags, indexer.Tag())
	}
	return idxCtx
}

func (idxCtx *IndexContext) ParseTxn() {
	idxCtx.ParseSpends()
	idxCtx.ParseTxos()
}

func (idxCtx *IndexContext) ParseSpends() {
	if idxCtx.Tx.IsCoinbase() {
		return
	}
	for _, txin := range idxCtx.Tx.Inputs {
		outpoint := lib.NewOutpointFromHash(txin.SourceTXID, txin.SourceTxOutIndex)
		var spend *Txo

		if txo, err := idxCtx.LoadTxo(outpoint); err != nil {
			log.Panic(err)
		} else {
			spend = txo
		}
		if spend == nil {
			spend = &Txo{
				Outpoint: outpoint,
				Data:     make(map[string]*IndexData),
			}
		}
		idxCtx.Spends = append(idxCtx.Spends, spend)
	}
}

func (idxCtx *IndexContext) ParseTxos() {
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
			txo.AddOwner(pkhash.Address())
		}
		idxCtx.Txos = append(idxCtx.Txos, txo)
		accSats += txout.Satoshis
		for _, indexer := range idxCtx.Indexers {
			if data := indexer.Parse(idxCtx, uint32(vout)); data != nil {
				txo.Data[indexer.Tag()] = data
			}
		}
	}
}

func (idxCtx *IndexContext) LoadTxo(outpoint *lib.Outpoint) (txo *Txo, err error) {
	op := outpoint.String()
	if txo, err = LoadTxo(idxCtx.Ctx, op, idxCtx.tags); err != nil {
		log.Panic(err)
		return nil, err
	} else if txo != nil {
		for i, indexer := range idxCtx.Indexers {
			tag := idxCtx.tags[i]
			if data, ok := txo.Data[tag]; ok {
				if data.Data, err = indexer.FromBytes(data.Data.(json.RawMessage)); err != nil {
					log.Panic(err)
					return nil, err
				}
			}
		}
	} else if idxCtx.ancestorConfig.Parse {
		parentTxid := outpoint.TxidHex()
		if tx, err := jb.LoadTx(idxCtx.Ctx, parentTxid, true); err != nil {
			log.Panicln(err)
			return nil, err
		} else {
			spendCtx := NewIndexContext(idxCtx.Ctx, tx, idxCtx.Indexers, AncestorConfig{})
			spendCtx.ParseTxos()
			txo = spendCtx.Txos[outpoint.Vout()]
			if idxCtx.ancestorConfig.Save {
				if err := txo.Save(idxCtx.Ctx, spendCtx.Height, spendCtx.Idx); err != nil {
					log.Panic(err)
					return nil, err
				}
			}
		}
	}
	return txo, nil
}

func (idxCtx *IndexContext) Save() error {
	if err := idxCtx.SaveTxos(); err != nil {
		log.Panic(err)
		return err
	} else if err := idxCtx.SaveSpends(); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func (idxCtx *IndexContext) SaveTxos() error {
	for _, indexer := range idxCtx.Indexers {
		indexer.PreSave(idxCtx)
	}
	for _, txo := range idxCtx.Txos {
		if err := txo.Save(idxCtx.Ctx, idxCtx.Height, idxCtx.Idx); err != nil {
			return err
		}
	}
	return nil
}

func (idxCtx *IndexContext) SaveSpends() error {
	for _, spend := range idxCtx.Spends {
		if err := spend.SaveSpend(idxCtx.Ctx, idxCtx.TxidHex, idxCtx.Height, idxCtx.Idx); err != nil {
			return err
		}
	}
	return nil
}
