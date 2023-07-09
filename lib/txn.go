package lib

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"log"
	"math"

	"github.com/libsv/go-bt/v2"
)

const THREADS = 16

type IndexContext struct {
	Txid          ByteString        `json:"txid"`
	Height        uint32            `json:"height"`
	Idx           uint64            `json:"idx"`
	Txos          []*Txo            `json:"txos"`
	ParsedScripts []*ParsedScript   `json:"parsed"`
	Inscriptions  []*ParsedScript   `json:"inscriptions"`
	Spends        []*Txo            `json:"spends"`
	Listings      []*OrdLockListing `json:"listings"`
	Bsv20s        []*Bsv20          `json:"bsv20s"`
	MissingInputs bool
}

func IndexSpends(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
	var accSatsIn uint64
	var err error
	for vin, txin := range tx.Inputs {
		spend := &Txo{
			Txid:  txin.PreviousTxID(),
			Vout:  txin.PreviousTxOutIndex,
			Spend: ctx.Txid,
			Vin:   uint32(vin),
		}
		ctx.Spends = append(ctx.Spends, spend)

		exists := spend.SaveSpend()
		if !exists {
			var tx *bt.Tx
			hexId := hex.EncodeToString(spend.Txid)
			tx, err = LoadTx(hexId)
			if err != nil {
				if ctx.Height > 0 && ctx.Height < uint32(math.Pow(2, 31)-1) {
					log.Panicf("%x: %d %v\n", spend.Txid, ctx.Height, err)
				}
				ctx.MissingInputs = true
				log.Printf("Missing Inputs %x\n", spend.Txid)
				continue
			}

			accSatsOut := uint64(0)
			for vout, txout := range tx.Outputs {
				if vout < int(spend.Vout) {
					spend.AccSats += txout.Satoshis
				}
				if _, err = InsBareSpend.Exec(
					spend.Txid,
					spend.Vout,
					txout.Satoshis,
					accSatsOut,
					spend.Spend,
					spend.Vin,
				); err != nil {
					log.Panicf("%x: %d %v\n", spend.Txid, ctx.Height, err)
				}
				accSatsOut += txout.Satoshis
			}
			spend.Satoshis = tx.Outputs[spend.Vout].Satoshis
		}

		accSatsIn += spend.Satoshis
		spend.AccSats = accSatsIn
		if Rdb != nil {
			outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
			msg := outpoint.String()
			if len(spend.Lock) > 0 {
				Rdb.Publish(context.Background(), hex.EncodeToString(spend.Lock), msg)
			}
			if spend.Listing {
				Rdb.Publish(context.Background(), "unlist", msg)
			}
		}
	}
}

func IndexTxos(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
	accSats := uint64(0)
	for vout, txout := range tx.Outputs {
		accSats += txout.Satoshis
		outpoint := Outpoint(binary.BigEndian.AppendUint32(ctx.Txid, uint32(vout)))
		txo := &Txo{
			Txid:     ctx.Txid,
			Vout:     uint32(vout),
			Height:   ctx.Height,
			Idx:      ctx.Idx,
			Satoshis: txout.Satoshis,
			AccSats:  accSats,
			Outpoint: &outpoint,
		}

		var accSpendSats uint64
		if !ctx.MissingInputs {
			for _, spend := range ctx.Spends {
				accSpendSats += spend.Satoshis
				if txo.Satoshis == 1 && spend.Satoshis == 1 && accSpendSats == txo.AccSats {
					txo.Origin = spend.Origin
					txo.PrevOrd = spend
				}
			}
		}

		if txo.Satoshis == 1 {
			parsed := ParseScript(*txout.LockingScript, tx, ctx.Height)
			txo.Lock = parsed.Lock
			if !ctx.MissingInputs && txo.Origin == nil && parsed.Ord != nil {
				txo.Origin = txo.Outpoint
			}
			if parsed.Listing != nil {
				txo.Listing = true
			}
			if parsed.Bsv20 != nil {
				txo.Bsv20 = parsed.Bsv20.Op != "deploy"
				bsv20 := parsed.Bsv20
				bsv20.Txid = ctx.Txid
				bsv20.Vout = uint32(vout)
				bsv20.Height = ctx.Height
				bsv20.Idx = ctx.Idx
				bsv20.Lock = parsed.Lock
				bsv20.Map = parsed.Map
				bsv20.B = parsed.B
				bsv20.Listing = parsed.Listing != nil
				ctx.Bsv20s = append(ctx.Bsv20s, bsv20)
			}
			ctx.Txos = append(ctx.Txos, txo)

			if txo.Origin != nil {
				parsed.Txid = ctx.Txid
				parsed.Vout = uint32(vout)
				parsed.Height = ctx.Height
				parsed.Idx = ctx.Idx
				parsed.Origin = txo.Origin
				if txo.Origin == &outpoint {
					ctx.Inscriptions = append(ctx.Inscriptions, parsed)
				}
				ctx.ParsedScripts = append(ctx.ParsedScripts, parsed)

				if parsed.Listing != nil {
					parsed.Listing.Txid = ctx.Txid
					parsed.Listing.Vout = uint32(vout)
					parsed.Listing.Origin = txo.Origin
					parsed.Listing.Height = ctx.Height
					parsed.Listing.Idx = ctx.Idx
					parsed.Listing.Outpoint = &outpoint
					ctx.Listings = append(ctx.Listings, parsed.Listing)
				}
			}
		}
	}
	if !dryRun {
		for _, txo := range ctx.Txos {
			impliedBsv20 := false
			if len(ctx.Bsv20s) == 0 && txo.PrevOrd != nil {
				impliedBsv20 = txo.PrevOrd.Bsv20
				txo.Bsv20 = txo.PrevOrd.Bsv20
			}
			txo.Save()
			if Rdb != nil {
				Rdb.Publish(context.Background(), hex.EncodeToString(txo.Lock), txo.Outpoint.String())
			}
			if impliedBsv20 {
				saveImpliedBsv20Transfer(txo.PrevOrd.Txid, txo.PrevOrd.Vout, txo)
			}
		}
		for _, inscription := range ctx.Inscriptions {
			inscription.SaveInscription()
		}
		for _, parsed := range ctx.ParsedScripts {
			parsed.Save()
		}
		for _, listing := range ctx.Listings {
			listing.Save()
			if Rdb != nil {
				Rdb.Publish(context.Background(), "list", listing.Outpoint.String())
			}
		}
		hasTransfer := false
		for _, bsv20 := range ctx.Bsv20s {
			if bsv20.Op == "transfer" {
				hasTransfer = true
			}
			bsv20.Save()
		}

		if hasTransfer && ctx.Height == 4294967295 {
			ValidateTransfer(ctx.Txid)
		}
	}
}

func IndexTxn(tx *bt.Tx, height uint32, idx uint64, dryRun bool) (ctx *IndexContext, err error) {
	txid := tx.TxIDBytes()
	ctx = &IndexContext{
		Txid:   txid,
		Height: height,
		Idx:    idx,
	}

	if height == 0 {
		// Set height to max uint32 so that it sorts to the end of the list
		ctx.Height = uint32(math.Pow(2, 31) - 1)
	}

	if !tx.IsCoinbase() {
		ctx.Spends = make([]*Txo, 0, len(tx.Inputs))
		IndexSpends(tx, ctx, dryRun)
	} else {
		ctx.Spends = make([]*Txo, 0)
	}
	IndexTxos(tx, ctx, dryRun)

	// wg.Wait()
	return
}
