package lib

import (
	"context"
	"log"

	"github.com/bitcoin-sv/go-sdk/chainhash"
	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/redis/go-redis/v9"
)

type IndexContext struct {
	Id       uint64                   `json:"id"`
	Tx       *transaction.Transaction `json:"-"`
	Txid     *chainhash.Hash          `json:"txid"`
	Height   uint32                   `json:"height"`
	Idx      uint64                   `json:"idx"`
	Txos     []*Txo                   `json:"txos"`
	Spends   []*Txo                   `json:"spends"`
	Indexers []Indexer                `json:"-"`
}

func (idxCtx *IndexContext) ParseTxn(ctx context.Context) {
	if !idxCtx.Tx.IsCoinbase() {
		idxCtx.ParseSpends(ctx)
	}

	idxCtx.ParseTxos()
}

func (idxCtx *IndexContext) ParseTxos() {
	accSats := uint64(0)
	for vout, txout := range idxCtx.Tx.Outputs {
		outpoint := NewOutpointFromHash(idxCtx.Txid, uint32(vout))
		txo := &Txo{
			Outpoint: outpoint,
			Height:   idxCtx.Height,
			Idx:      idxCtx.Idx,
			Satoshis: txout.Satoshis,
			OutAcc:   accSats,
			Data:     make(map[string]*IndexData),
		}
		if len(*txout.LockingScript) >= 25 && script.NewFromBytes((*txout.LockingScript)[:25]).IsP2PKH() {
			pkhash := PKHash((*txout.LockingScript)[3:23])
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

func (idxCtx *IndexContext) ParseSpends(ctx context.Context) {
	for _, txin := range idxCtx.Tx.Inputs {
		outpoint := NewOutpointFromHash(txin.SourceTXID, txin.SourceTxOutIndex)
		if spend, err := idxCtx.LoadTxo(ctx, outpoint); err != nil {
			log.Panic(err)
		} else {
			idxCtx.Spends = append(idxCtx.Spends, spend)
		}
	}
}

func (idxCtx *IndexContext) Save(ctx context.Context) error {
	txid := idxCtx.Txid.String()
	score := HeightScore(idxCtx.Height, idxCtx.Idx)
	if err := Rdb.ZAdd(ctx, TxStatusKey, redis.Z{
		Score:  -score,
		Member: txid,
	}).Err(); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
		return err
	} else if err := idxCtx.SaveTxos(ctx); err != nil {
		log.Panic(err)
		return err
	} else if err := idxCtx.SaveSpends(ctx); err != nil {
		log.Panic(err)
		return err
	}

	if err := Rdb.ZAdd(ctx, TxStatusKey, redis.Z{
		Score:  score,
		Member: txid,
	}).Err(); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
		return err
	}
	return nil
}

func (idxCtx *IndexContext) LoadTxo(ctx context.Context, outpoint *Outpoint) (txo *Txo, err error) {
	if txo, err = LoadTxo(ctx, outpoint); err != nil {
		log.Panic(err)
		return nil, err
	} else if txo == nil {
		if tx, err := LoadTx(ctx, outpoint.TxidHex()); err != nil {
			log.Panicln(err)
			return nil, err
		} else {
			spendCtx := NewIndexContext(tx, idxCtx.Indexers)
			spendCtx.ParseTxos()
			if err := spendCtx.SaveTxos(ctx); err != nil {
				log.Panic(err)
				return nil, err
			}
			txo = spendCtx.Txos[outpoint.Vout()]
		}
	}
	return txo, nil
}

func (idxCtx *IndexContext) SaveTxos(ctx context.Context) error {
	for _, indexer := range idxCtx.Indexers {
		indexer.PreSave(idxCtx)
	}
	for _, txo := range idxCtx.Txos {
		if err := txo.Save(ctx, idxCtx); err != nil {
			return err
		}
	}
	return nil
}

func (idxCtx *IndexContext) SaveSpends(ctx context.Context) error {
	for _, spend := range idxCtx.Spends {
		if err := spend.SaveSpend(ctx, idxCtx); err != nil {
			return err
		}
	}
	return nil
}
