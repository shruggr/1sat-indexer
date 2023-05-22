package lib

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"log"
	"math"

	"github.com/libsv/go-bt/v2"
)

const THREADS = 16

type Status uint

var (
	Unconfirmed Status = 0
	Confirmed   Status = 1
)

type IndexResult struct {
	Txid          ByteString      `json:"txid"`
	Height        uint32          `json:"height"`
	Idx           uint64          `json:"idx"`
	Txos          []*Txo          `json:"txos"`
	ParsedScripts []*ParsedScript `json:"parsed"`
	Spends        []*Txo          `json:"spends"`
}

func IndexTxn(tx *bt.Tx, height uint32, idx uint64) (result *IndexResult, err error) {
	txid := tx.TxIDBytes()
	result = &IndexResult{
		Txid:   txid,
		Height: height,
		Idx:    idx,
		Spends: make([]*Txo, len(tx.Inputs)),
		Txos:   make([]*Txo, len(tx.Outputs)),
	}
	var accSats uint64
	if height == 0 {
		// Set height to max uint32 so that it sorts to the end of the list
		height = uint32(math.Pow(2, 31) - 1)
	}
	// threadLimiter := make(chan struct{}, THREADS)
	// var wg sync.WaitGroup
	for vin, txin := range tx.Inputs {
		// threadLimiter <- struct{}{}
		// wg.Add(1)
		// go func(txin *bt.Input, vin int) {
		spend := &Txo{
			Txid:  txin.PreviousTxID(),
			Vout:  txin.PreviousTxOutIndex,
			Spend: txid,
			Vin:   uint32(vin),
		}
		result.Spends[vin] = spend

		var exists bool
		exists, err = spend.SaveSpend()
		if err != nil {
			log.Panic(err)
			return
		}
		if !exists {
			var tx *bt.Tx
			tx, err = LoadTx(spend.Txid)
			if err != nil {
				log.Panicf("%x: %v\n", spend.Txid, err)
			}
			for vout, txout := range tx.Outputs {
				if vout > int(spend.Vout) {
					break
				}
				spend.AccSats += txout.Satoshis
			}
			spend.Satoshis = tx.Outputs[spend.Vout].Satoshis
			hash := sha256.Sum256(*tx.Outputs[spend.Vout].LockingScript)
			spend.Lock = bt.ReverseBytes(hash[:])

			err = spend.SaveWithSpend()
			if err != nil {
				log.Panic(err)
			}
		}

		accSats += spend.Satoshis
		spend.AccSats = accSats
		outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))

		msg := outpoint.String()
		Rdb.Publish(context.Background(), hex.EncodeToString(spend.Lock), msg)
		if spend.Listing {
			Rdb.Publish(context.Background(), "unlist", msg)
		}
		// 	wg.Done()
		// 	<-threadLimiter
		// }(txin, vin)
	}
	// wg.Wait()

	accSats = 0
	for vout, txout := range tx.Outputs {
		accSats += txout.Satoshis
		txo := &Txo{
			Txid:     txid,
			Vout:     uint32(vout),
			Height:   height,
			Idx:      idx,
			Satoshis: txout.Satoshis,
			AccSats:  accSats,
		}

		var accSpendSats uint64
		for _, spend := range result.Spends {
			accSpendSats += spend.Satoshis
			if spend.Satoshis == 1 && accSpendSats == txo.AccSats {
				txo.Origin = spend.Origin
				txo.Bsv20 = spend.Bsv20
			}
		}

		// threadLimiter <- struct{}{}
		// wg.Add(1)
		// go func(txo *Txo, txout *bt.Output, vout int) {
		outpoint := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
		msg := outpoint.String()
		parsed := ParseScript(*txout.LockingScript, tx, height)
		txo.Lock = parsed.Lock
		if txo.Origin == nil && parsed.Ord != nil && txo.Satoshis == 1 {
			txo.Origin = &outpoint
		}
		if parsed.Listing != nil {
			txo.Listing = true
		}
		if parsed.Bsv20 != nil {
			txo.Bsv20 = true
		}
		result.Txos = append(result.Txos, txo)
		err = txo.Save()
		if err != nil {
			log.Panicf("%x: %v\n", txid, err)
		}
		Rdb.Publish(context.Background(), hex.EncodeToString(txo.Lock), msg)
		if txo.Origin != nil {
			parsed.Txid = txid
			parsed.Vout = uint32(vout)
			parsed.Height = height
			parsed.Idx = idx
			parsed.Origin = txo.Origin
			if txo.Origin == &outpoint {
				err = parsed.SaveInscription()
				if err != nil {
					log.Panic(err)
				}
			}
			err = parsed.Save()
			if err != nil {
				log.Panicf("%x: %v\n", txid, err)
			}

			if parsed.Listing != nil {
				parsed.Listing.Txid = txid
				parsed.Listing.Vout = uint32(vout)
				parsed.Listing.Origin = txo.Origin
				parsed.Listing.Height = height
				parsed.Listing.Idx = idx
				err = parsed.Listing.Save()
				if err != nil {
					log.Panicf("%x: %v\n", txid, err)
				}
				Rdb.Publish(context.Background(), "list", msg)
			}
			result.ParsedScripts = append(result.ParsedScripts, parsed)
		}
		if parsed.Bsv20 != nil {
			bsv20 := parsed.Bsv20
			bsv20.Txid = txid
			bsv20.Vout = uint32(vout)
			bsv20.Height = height
			bsv20.Idx = uint64(idx)
			bsv20.Lock = parsed.Lock
			bsv20.Map = parsed.Map
			bsv20.B = parsed.B
			bsv20.Save()
		} else if txo.Bsv20 {
			saveImpliedBsv20Transfer(txid, uint32(vout), txo)
		}
		// wg.Done()
		// <-threadLimiter
		// }(txo, txout, vout)
	}
	// wg.Wait()
	return
}
