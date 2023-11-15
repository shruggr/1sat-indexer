package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"log"

	"github.com/libsv/go-bt/v2"
	"github.com/shruggr/1sat-indexer/lib"
)

type IndexContext struct {
	Txid    []byte        `json:"txid"`
	BlockId *string       `json:"blockId"`
	Height  *uint32       `json:"height"`
	Idx     uint64        `json:"idx"`
	Txos    []*lib.Txo    `json:"txos"`
	Origins []*lib.Origin `json:"origin"`
	Spends  []*lib.Spend  `json:"spends"`
}

func IndexSpends(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
	var accSatsIn uint64
	var err error
	for vin, txin := range tx.Inputs {
		spend := &lib.Spend{
			Txid:   txin.PreviousTxID(),
			Vout:   txin.PreviousTxOutIndex,
			Spend:  ctx.Txid,
			Vin:    uint32(vin),
			Height: ctx.Height,
			Idx:    ctx.Idx,
		}
		spend.Outpoint = lib.NewOutpoint(spend.Txid, spend.Vout)
		ctx.Spends = append(ctx.Spends, spend)

		exists := spend.SetSpent()
		if !exists {
			var tx *bt.Tx
			hexId := hex.EncodeToString(spend.Txid)
			tx, err = lib.LoadTx(hexId)
			// txout, err := LoadTxOut(hexId, spend.Vout)
			if err != nil {
				if ctx.Height != nil {
					log.Panicf("%x: %d %v\n", spend.Txid, ctx.Height, err)
				}
				spend.Missing = true
				log.Printf("Missing Inputs %x\n", spend.Txid)
				continue
			}

			for vout, txout := range tx.Outputs {
				if vout < int(spend.Vout) {
					spend.OutAcc += txout.Satoshis
					continue
				}
				spend.Satoshis = txout.Satoshis
				spend.Save()
				break
			}
			spend.Satoshis = tx.Outputs[spend.Vout].Satoshis
		}

		spend.InAcc = accSatsIn
		accSatsIn += spend.Satoshis
		if rdb != nil {
			outpoint := lib.Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
			msg := outpoint.String()
			if len(spend.PKHash) > 0 {
				rdb.Publish(context.Background(), hex.EncodeToString(spend.PKHash), msg)
			}
			if spend.Data != nil && spend.Data.Listing != nil {
				rdb.Publish(context.Background(), "unlist", msg)
			}
		}
	}
}

func IndexTxos(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
	accSats := uint64(0)
	for vout, txout := range tx.Outputs {
		if txout.Satoshis != 1 {
			accSats += txout.Satoshis
			continue
		}
		outpoint := lib.Outpoint(binary.BigEndian.AppendUint32(ctx.Txid, uint32(vout)))
		txo := &lib.Txo{
			Tx:       tx,
			Txid:     ctx.Txid,
			Vout:     uint32(vout),
			Height:   ctx.Height,
			Idx:      ctx.Idx,
			Satoshis: txout.Satoshis,
			OutAcc:   accSats,
			Outpoint: &outpoint,
		}

		for vin, spend := range ctx.Spends {
			if spend.Missing {
				log.Printf("Missing Inputs %x\n", txo.Txid)
				return
			}
			if spend.InAcc < txo.OutAcc && len(ctx.Spends) > vin+1 {
				continue
			} else if spend.InAcc == txo.OutAcc && spend.Satoshis == 1 {
				txo.Origin = spend.Origin
				txo.Spend = spend
				break
			}
			txo.IsOrigin = true
			break
		}

		if txo.IsOrigin {
			origin := &lib.Origin{
				Origin: txo.Outpoint,
				Txid:   ctx.Txid,
				Vout:   txo.Vout,
				Height: ctx.Height,
				Idx:    ctx.Idx,
				Data:   txo.Data,
				Map:    txo.Data.Map,
			}
			ctx.Origins = append(ctx.Origins, origin)
			txo.Origin = txo.Outpoint
		}
		ctx.Txos = append(ctx.Txos, txo)
		accSats += txout.Satoshis
	}
	if !dryRun {
		for _, origin := range ctx.Origins {
			origin.Save()
		}

		for _, txo := range ctx.Txos {
			if rdb != nil {
				rdb.Publish(context.Background(), hex.EncodeToString(txo.PKHash), txo.Outpoint.String())
			}
			txo.SetOrigin()

			if txo.Data == nil {
				continue
			}
		}
	}
}

func IndexTxn(tx *bt.Tx, blockId *string, height *uint32, idx uint64, dryRun bool) (ctx *IndexContext, err error) {
	txid := tx.TxIDBytes()
	ctx = &IndexContext{
		Txid:    txid,
		BlockId: blockId,
		Height:  height,
		Idx:     idx,
		Spends:  make([]*lib.Spend, 0, len(tx.Inputs)),
	}

	if !tx.IsCoinbase() {
		IndexSpends(tx, ctx, dryRun)
	}

	IndexTxos(tx, ctx, dryRun)

	return
}
