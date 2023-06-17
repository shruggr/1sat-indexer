package lib

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/libsv/go-bt/v2"
)

const THREADS = 16

// type IndexResult struct {
// 	Txos          []*Txo          `json:"txos"`
// 	ParsedScripts []*ParsedScript `json:"parsed"`
// 	Spends        []*Txo          `json:"spends"`
// }

type IndexResult struct {
	Txid ByteString `json:"txid"`
	// Height        uint32            `json:"height"`
	// Idx           uint64            `json:"idx"`
	Txos          []*Txo            `json:"txos"`
	Files         []*File           `json:"files"`
	ParsedScripts []*ParsedScript   `json:"parsed"`
	Inscriptions  []*ParsedScript   `json:"inscriptions"`
	Spends        []*Txo            `json:"spends"`
	Listings      []*OrdLockListing `json:"listings"`
	Bsv20s        []*Bsv20          `json:"bsv20s"`
}

func FullIndexTxn(tx *bt.Tx, height uint32, idx uint64, dryRun bool) (fees uint64, err error) {
	result := &IndexResult{}
	spendsAcc := map[uint64]*Txo{}
	txid := tx.TxIDBytes()
	var satsIn uint64
	if !tx.IsCoinbase() {
		for vin, txin := range tx.Inputs {
			spend := &Txo{
				Txid:  txin.PreviousTxID(),
				Vout:  txin.PreviousTxOutIndex,
				Spend: txid,
				InAcc: satsIn,
				Vin:   uint32(vin),
				Flags: map[string]bool{},
			}
			spend.SaveSpend()
			spendsAcc[satsIn] = spend

			satsIn += spend.Satoshis
			// spend.OutAcc = satsIn

			result.Spends = append(result.Spends, spend)
			// outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
			// msg := outpoint.String()
			// Rdb.Publish(context.Background(), hex.EncodeToString(spend.Lock), msg)
		}
	}

	var satsOut uint64
	for vout, txout := range tx.Outputs {
		if txout.Satoshis == 0 {
			continue
		}

		scripthash := sha256.Sum256(*txout.LockingScript)
		txo := &Txo{
			Txid:       txid,
			Vout:       uint32(vout),
			Satoshis:   txout.Satoshis,
			OutAcc:     satsOut,
			Flags:      map[string]bool{},
			Scripthash: scripthash[:],
		}

		outpoint := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
		txo.Outpoint = &outpoint

		if txout.Satoshis == 1 {
			for _, spend := range result.Spends {
				if spend.Satoshis == 1 && spend.InAcc == txo.OutAcc {
					txo.Origin = spend.Origin
					txo.InOrd = spend
				}
			}
			parsed := ParseScript(*txout.LockingScript, tx, height)

			if txo.Origin == nil && parsed.Inscription != nil && txo.Satoshis == 1 {
				txo.Origin = txo.Outpoint
			}

			if parsed.Listing != nil {
				txo.Flags["list"] = true
			}
			// if parsed.Bsv20 != nil {
			// 	if parsed.Bsv20.Op != "deploy" {
			// 		txo.Flags["bsv20"] = true
			// 	}
			// 	bsv20 := parsed.Bsv20
			// 	bsv20.Txid = txid
			// 	bsv20.Vout = uint32(vout)
			// 	bsv20.Height = height
			// 	bsv20.Idx = uint64(idx)
			// 	bsv20.Lock = parsed.Lock
			// 	bsv20.Map = parsed.Map
			// 	bsv20.B = parsed.B
			// 	bsv20.Listing = parsed.Listing != nil
			// 	result.Bsv20s = append(result.Bsv20s, bsv20)
			// }

			if parsed.Inscription != nil {
				// txo.Flags["ord"] = true
				parsed.Txid = txid
				parsed.Vout = uint32(vout)
				parsed.Height = height
				parsed.Idx = idx
				result.Inscriptions = append(result.Inscriptions, parsed)

			}
			result.ParsedScripts = append(result.ParsedScripts, parsed)

			if parsed.Listing != nil {
				parsed.Listing.Txid = txid
				parsed.Listing.Vout = uint32(vout)
				parsed.Listing.Origin = txo.Origin
				parsed.Listing.Height = height
				parsed.Listing.Idx = idx
				parsed.Listing.Outpoint = &outpoint
				result.Listings = append(result.Listings, parsed.Listing)
			}
		}

		result.Txos = append(result.Txos, txo)
		satsOut += txout.Satoshis
		// if !dryRun && txo.Satoshis > 0 {
		// 	txo.Save()
		// 	if err != nil {
		// 		return
		// 	}
		// 	// Rdb.Publish(context.Background(), hex.EncodeToString(txo.Lock), msg)
		// }
	}

	if !dryRun {
		for _, txo := range result.Txos {
			impliedBsv20 := false
			if len(result.Bsv20s) == 0 && txo.PrevOrd != nil {
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
		for _, inscription := range result.Inscriptions {
			inscription.SaveInscription()
		}
		for _, parsed := range result.ParsedScripts {
			parsed.Save()
		}
		for _, listing := range result.Listings {
			listing.Save()
			if Rdb != nil {
				Rdb.Publish(context.Background(), "list", listing.Outpoint.String())
			}
		}
		for _, bsv20 := range result.Bsv20s {
			bsv20.Save()
		}
	}
	if !tx.IsCoinbase() {
		fees = satsIn - satsOut
	}
	return
}

func SaveTxn(txid []byte, height uint32, idx uint64, fees uint64, accFees uint64) (err error) {
	_, err = SetTxn.Exec(txid, height, idx, fees, accFees)
	return
}

// type IndexResult struct {
// 	Txid          ByteString        `json:"txid"`
// 	Height        uint32            `json:"height"`
// 	Idx           uint64            `json:"idx"`
// 	Txos          []*Txo            `json:"txos"`
// 	ParsedScripts []*ParsedScript   `json:"parsed"`
// 	Inscriptions  []*ParsedScript   `json:"inscriptions"`
// 	Spends        []*Txo            `json:"spends"`
// 	Listings      []*OrdLockListing `json:"listings"`
// 	Bsv20s        []*Bsv20          `json:"bsv20s"`
// }

// func IndexTxn(tx *bt.Tx, height uint32, idx uint64, dryRun bool) (result *IndexResult, err error) {
// 	txid := tx.TxIDBytes()
// 	result = &IndexResult{
// 		Txid:   txid,
// 		Height: height,
// 		Idx:    idx,
// 		Spends: make([]*Txo, len(tx.Inputs)),
// 		// Txos:   make([]*Txo, len(tx.Outputs)),
// 	}
// 	var accSats uint64
// 	if height == 0 {
// 		// Set height to max uint32 so that it sorts to the end of the list
// 		height = uint32(math.Pow(2, 31) - 1)
// 	}
// 	// threadLimiter := make(chan struct{}, THREADS)
// 	// var wg sync.WaitGroup
// 	for vin, txin := range tx.Inputs {
// 		// threadLimiter <- struct{}{}
// 		// wg.Add(1)
// 		// go func(txin *bt.Input, vin int) {
// 		spend := &Txo{
// 			Txid:  txin.PreviousTxID(),
// 			Vout:  txin.PreviousTxOutIndex,
// 			Spend: txid,
// 			Vin:   uint32(vin),
// 		}
// 		result.Spends[vin] = spend

// 		exists := spend.SaveSpend()
// 		if !exists {
// 			tx := bt.NewTx()
// 			if height == uint32(math.Pow(2, 31)-1) {
// 				r, err := bit.GetRawTransactionRest(hex.EncodeToString(spend.Txid))
// 				if err != nil {
// 					log.Panicf("%x: %d %v\n", spend.Txid, height, err)
// 				}
// 				if _, err = tx.ReadFrom(r); err != nil {
// 					log.Panicf("%x: %v\n", spend.Txid, err)
// 				}
// 			} else {
// 				tx, err = LoadTx(spend.Txid)
// 				if err != nil {
// 					log.Panicf("%x: %v\n", spend.Txid, err)
// 				}
// 			}
// 			for vout, txout := range tx.Outputs {
// 				if vout > int(spend.Vout) {
// 					break
// 				}
// 				spend.AccSats += txout.Satoshis
// 			}
// 			spend.Satoshis = tx.Outputs[spend.Vout].Satoshis
// 			hash := sha256.Sum256(*tx.Outputs[spend.Vout].LockingScript)
// 			spend.Lock = bt.ReverseBytes(hash[:])

// 			spend.SaveWithSpend()
// 		}

// 		accSats += spend.Satoshis
// 		spend.AccSats = accSats
// 		if Rdb != nil {
// 			outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
// 			msg := outpoint.String()
// 			Rdb.Publish(context.Background(), hex.EncodeToString(spend.Lock), msg)
// 			if spend.Listing {
// 				Rdb.Publish(context.Background(), "unlist", msg)
// 			}

// 		}
// 		// 	wg.Done()
// 		// 	<-threadLimiter
// 		// }(txin, vin)
// 	}
// 	// wg.Wait()

// 	accSats = 0
// 	for vout, txout := range tx.Outputs {
// 		accSats += txout.Satoshis
// 		outpoint := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
// 		txo := &Txo{
// 			Txid:     txid,
// 			Vout:     uint32(vout),
// 			Height:   height,
// 			Idx:      idx,
// 			Satoshis: txout.Satoshis,
// 			AccSats:  accSats,
// 			Outpoint: &outpoint,
// 		}

// 		var accSpendSats uint64
// 		for _, spend := range result.Spends {
// 			accSpendSats += spend.Satoshis
// 			if txo.Satoshis == 1 && spend.Satoshis == 1 && accSpendSats == txo.AccSats {
// 				txo.Origin = spend.Origin
// 				txo.PrevOrd = spend
// 			}
// 		}

// 		// threadLimiter <- struct{}{}
// 		// wg.Add(1)
// 		// go func(txo *Txo, txout *bt.Output, vout int) {
// 		// msg := outpoint.String()
// 		parsed := ParseScript(*txout.LockingScript, tx, height)
// 		txo.Lock = parsed.Lock
// 		if txo.Origin == nil && parsed.Ord != nil && txo.Satoshis == 1 {
// 			txo.Origin = txo.Outpoint
// 		}
// 		if parsed.Listing != nil {
// 			txo.Listing = true
// 		}
// 		if parsed.Bsv20 != nil {
// 			txo.Bsv20 = parsed.Bsv20.Op != "deploy"
// 			bsv20 := parsed.Bsv20
// 			bsv20.Txid = txid
// 			bsv20.Vout = uint32(vout)
// 			bsv20.Height = height
// 			bsv20.Idx = uint64(idx)
// 			bsv20.Lock = parsed.Lock
// 			bsv20.Map = parsed.Map
// 			bsv20.B = parsed.B
// 			bsv20.Listing = parsed.Listing != nil
// 			result.Bsv20s = append(result.Bsv20s, bsv20)
// 		}
// 		result.Txos = append(result.Txos, txo)

// 		if txo.Origin != nil {
// 			parsed.Txid = txid
// 			parsed.Vout = uint32(vout)
// 			parsed.Height = height
// 			parsed.Idx = idx
// 			parsed.Origin = txo.Origin
// 			if txo.Origin == &outpoint {
// 				result.Inscriptions = append(result.Inscriptions, parsed)
// 			}
// 			result.ParsedScripts = append(result.ParsedScripts, parsed)

// 			if parsed.Listing != nil {
// 				parsed.Listing.Txid = txid
// 				parsed.Listing.Vout = uint32(vout)
// 				parsed.Listing.Origin = txo.Origin
// 				parsed.Listing.Height = height
// 				parsed.Listing.Idx = idx
// 				parsed.Listing.Outpoint = &outpoint
// 				result.Listings = append(result.Listings, parsed.Listing)
// 			}
// 		}

// 		// wg.Done()
// 		// <-threadLimiter
// 		// }(txo, txout, vout)
// 	}
// 	if !dryRun {
// 		for _, txo := range result.Txos {
// 			impliedBsv20 := false
// 			if len(result.Bsv20s) == 0 && txo.PrevOrd != nil {
// 				impliedBsv20 = txo.PrevOrd.Bsv20
// 				txo.Bsv20 = txo.PrevOrd.Bsv20
// 			}
// 			txo.Save()
// 			if Rdb != nil {
// 				Rdb.Publish(context.Background(), hex.EncodeToString(txo.Lock), txo.Outpoint.String())
// 			}
// 			if impliedBsv20 {
// 				saveImpliedBsv20Transfer(txo.PrevOrd.Txid, txo.PrevOrd.Vout, txo)
// 			}
// 		}
// 		for _, inscription := range result.Inscriptions {
// 			inscription.SaveInscription()
// 		}
// 		for _, parsed := range result.ParsedScripts {
// 			parsed.Save()
// 		}
// 		for _, listing := range result.Listings {
// 			listing.Save()
// 			if Rdb != nil {
// 				Rdb.Publish(context.Background(), "list", listing.Outpoint.String())
// 			}
// 		}
// 		for _, bsv20 := range result.Bsv20s {
// 			bsv20.Save()
// 		}
// 	}
// 	// wg.Wait()
// 	return
// }
