package lib

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"log"

	"github.com/libsv/go-bt/v2"
)

const THREADS = 64

type IndexContext struct {
	Txid    []byte  `json:"txid"`
	BlockId *string `json:"blockId"`
	Height  *uint32 `json:"height"`
	Idx     uint64  `json:"idx"`
	Txos    []*Txo  `json:"txos"`
	// ParsedScripts []*ParsedScript `json:"parsed"`
	Origins []*Origin `json:"origin"`
	// Inscriptions  []*Inscription  `json:"inscriptions"`
	Spends []*Spend `json:"spends"`
	// Listings      []*Listing      `json:"listings"`
	Bsv20s []*Txo `json:"bsv20s"`
}

func IndexSpends(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
	var accSatsIn uint64
	var err error
	for vin, txin := range tx.Inputs {
		spend := &Spend{
			Txid:   txin.PreviousTxID(),
			Vout:   txin.PreviousTxOutIndex,
			Spend:  ctx.Txid,
			Vin:    uint32(vin),
			Height: ctx.Height,
			Idx:    ctx.Idx,
		}
		ctx.Spends = append(ctx.Spends, spend)

		exists := spend.SetSpent()
		if !exists {
			var tx *bt.Tx
			hexId := hex.EncodeToString(spend.Txid)
			tx, err = LoadTx(hexId)
			if err != nil {
				if ctx.Height != nil {
					log.Panicf("%x: %d %v\n", spend.Txid, ctx.Height, err)
				}
				spend.Missing = true
				log.Printf("Missing Inputs %x\n", spend.Txid)
				continue
			}

			accSatsOut := uint64(0)
			for vout, txout := range tx.Outputs {
				if vout < int(spend.Vout) {
					spend.OutAcc += txout.Satoshis
					continue
				}
				spend.Satoshis = txout.Satoshis
				spend.Save()
				accSatsOut += txout.Satoshis
				break
			}
			spend.Satoshis = tx.Outputs[spend.Vout].Satoshis
		}

		spend.InAcc = accSatsIn
		accSatsIn += spend.Satoshis
		if Rdb != nil {
			outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
			msg := outpoint.String()
			if len(spend.PKHash) > 0 {
				Rdb.Publish(context.Background(), hex.EncodeToString(spend.PKHash), msg)
			}
			if spend.Data != nil && spend.Data.Listing != nil {
				Rdb.Publish(context.Background(), "unlist", msg)
			}
		}
	}
}

func IndexTxos(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
	accSats := uint64(0)
	for vout, txout := range tx.Outputs {
		outpoint := Outpoint(binary.BigEndian.AppendUint32(ctx.Txid, uint32(vout)))
		txo := &Txo{
			Tx:       tx,
			Txid:     ctx.Txid,
			Vout:     uint32(vout),
			Height:   ctx.Height,
			Idx:      ctx.Idx,
			Satoshis: txout.Satoshis,
			OutAcc:   accSats,
			Outpoint: &outpoint,
		}

		// if txo.Satoshis == 0 {
		// 	parsed := ParseScript(*txout.LockingScript, tx, ctx.Height)
		// 	ctx.ParsedScripts = append(ctx.ParsedScripts, parsed)
		// }
		if txo.Satoshis == 1 {
			for vin, spend := range ctx.Spends {
				if spend.Missing {
					log.Printf("Missing Inputs %x\n", txo.Txid)
					break
				}
				if spend.InAcc < txo.OutAcc && len(ctx.Spends) > vin+1 {
					continue
				} else if spend.InAcc == txo.OutAcc && spend.Satoshis == 1 {
					txo.Origin = spend.Origin
					txo.Spend = spend
					if ctx.Height != nil && *ctx.Height < 801000 {
						if spend.Data != nil && spend.Data.Bsv20 != nil &&
							(spend.Data.Bsv20.Op == "mint" || spend.Data.Bsv20.Op == "transfer") {
							txo.ImpliedBsv20 = true
						}
					}
					break
				}
				txo.IsOrigin = true
			}
			ParseScript(txo)
			if txo.IsOrigin && txo.Data != nil && txo.Data.Inscription != nil {
				origin := &Origin{
					Origin: txo.Outpoint,
					Txid:   ctx.Txid,
					Vout:   txo.Vout,
					Map:    txo.Data.Map,
					Height: *ctx.Height,
					Idx:    ctx.Idx,
				}
				ctx.Origins = append(ctx.Origins, origin)
				txo.Origin = txo.Outpoint
			}
			// if txo.Data.Listing != nil {
			// 	txo.Listing = true
			// }
			if txo.Data.Bsv20 != nil {
				// txo.Data.Types = append(txo.Data.Types, "bsv20")
				// txo.Bsv20 = parsed.Bsv20.Op != "deploy"
				// bsv20 := parsed.Bsv20
				// bsv20.Txid = ctx.Txid
				// bsv20.Vout = txo.Vout
				// bsv20.Height = ctx.Height
				// bsv20.Idx = ctx.Idx
				// bsv20.PKHash = parsed.PKHash
				// bsv20.Listing = parsed.Listing != nil
				ctx.Bsv20s = append(ctx.Bsv20s, txo)
			}
			// else if txo.Data.Inscription != nil {
			// 	txo.Data.Types = append(txo.Data.Types, "file")
			// 	// ins := parsed.Inscription
			// 	// ins.Txid = ctx.Txid
			// 	// ins.Vout = txo.Vout
			// 	// ins.Origin = txo.Origin
			// 	// ins.Height = ctx.Height
			// 	// ins.Idx = ctx.Idx
			// 	// ctx.Inscriptions = append(ctx.Inscriptions, ins)
			// }

			// if parsed.Listing != nil {
			// 	listing := parsed.Listing
			// 	listing.Txid = ctx.Txid
			// 	listing.Vout = txo.Vout
			// 	listing.Height = ctx.Height
			// 	listing.Idx = ctx.Idx
			// 	listing.Outpoint = txo.Outpoint
			// 	ctx.Listings = append(ctx.Listings, listing)
			// }
			// parsed.Txid = ctx.Txid
			// parsed.Vout = txo.Vout
			// parsed.Origin = txo.Origin
			// parsed.Height = ctx.Height
			// parsed.Idx = ctx.Idx
			// parsed.Outpoint = txo.Outpoint
			// ctx.ParsedScripts = append(ctx.ParsedScripts, parsed)

			ctx.Txos = append(ctx.Txos, txo)
			accSats += txout.Satoshis
		}
	}
	if !dryRun {
		_, err := Db.Exec(context.Background(), `
			INSERT INTO txns(txid, block_id, height, idx)
			VALUES($1, decode($2, 'hex'), $3, $4)
			ON CONFLICT(txid) DO UPDATE SET
				block_id=EXCLUDED.block_id,
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			ctx.Txid,
			ctx.BlockId,
			ctx.Height,
			ctx.Idx,
		)
		if err != nil {
			log.Panicf("%x %v\n", ctx.Txid, err)
		}
		for _, txo := range ctx.Txos {
			txo.Save()
			if Rdb != nil {
				Rdb.Publish(context.Background(), hex.EncodeToString(txo.PKHash), txo.Outpoint.String())
			}
			// Implied BSV20 transfer
			if len(ctx.Bsv20s) == 0 && txo.ImpliedBsv20 {
				txo.Data.Bsv20 = &Bsv20{
					Txid:   txo.Txid,
					Vout:   txo.Vout,
					Height: txo.Height,
					Idx:    txo.Idx,
					PKHash: txo.PKHash,
					Ticker: txo.Spend.Data.Bsv20.Ticker,
					Op:     "transfer",
					Amt:    txo.Spend.Data.Bsv20.Amt,
				}

				// saveImpliedBsv20Transfer(txo.Txid, txo.Vout, txo)
			}
		}
		for _, origin := range ctx.Origins {
			origin.Save()
		}
		// for _, inscription := range ctx.Inscriptions {
		// 	inscription.SaveInscription()
		// }
		// for _, p := range ctx.ParsedScripts {
		// 	if p.Map != nil {
		// 		p.SaveMap()
		// 		if ctx.Height != nil && p.Origin != nil && p.Outpoint != p.Origin {
		// 			p.UpdateMap()
		// 		}
		// 	}
		// 	// if p.B != nil {

		// 	// }
		// }
		// for _, listing := range ctx.Listings {
		// 	listing.Save()
		// 	if Rdb != nil {
		// 		Rdb.Publish(context.Background(), "list", listing.Outpoint.String())
		// 	}
		// }
		// hasTransfer := false
		// for _, txo := range ctx.Bsv20s {
		// 	if txo.Data.Bsv20.Op == "transfer" {
		// 		hasTransfer = true
		// 	}
		// 	txo.Data.Bsv20.Save()
		// }

		// if hasTransfer && ctx.Height == nil {
		// 	ValidateTransfer(ctx.Txid)
		// }
	}
}

func IndexTxn(tx *bt.Tx, blockId *string, height *uint32, idx uint64, dryRun bool) (ctx *IndexContext, err error) {
	txid := tx.TxIDBytes()
	ctx = &IndexContext{
		Txid:    txid,
		BlockId: blockId,
		Height:  height,
		Idx:     idx,
		Spends:  make([]*Spend, 0, len(tx.Inputs)),
	}

	if !tx.IsCoinbase() {
		IndexSpends(tx, ctx, dryRun)
	}

	IndexTxos(tx, ctx, dryRun)

	return
}
