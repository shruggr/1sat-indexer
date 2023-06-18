package lib

import (
	"crypto/sha256"
	"encoding/binary"
	"log"

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
	Txos []*Txo `json:"txos"`
	// Files         []*File           `json:"files"`
	// ParsedScripts []*ParsedScript `json:"parsed"`
	// Inscriptions  []*Inscription    `json:"inscriptions"`
	Spends []*Txo   `json:"spends"`
	Events []*Event `json:"events"`
	// Listings      []*OrdLockListing `json:"listings"`
	Bsv20s []*Bsv20 `json:"bsv20s"`
}

func IndexTxn(tx *bt.Tx, height uint32, idx uint64, dryRun bool) (fees uint64, err error) {
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
			}
			row := db.QueryRow(`UPDATE txos
				SET spend=$3, inacc=$4, vin=$5
				WHERE txid=$1 AND vout=$2
				RETURNING lock, satoshis, origin, bsv20, listing`,
				spend.Txid,
				spend.Vout,
				spend.Spend,
				spend.InAcc,
				spend.Vin,
			)

			err := row.Scan(&spend.Lock, &spend.Satoshis, &spend.Origin, &spend.Bsv20, &spend.Listing)
			if err != nil {
				log.Panic(err)
			}
			spendsAcc[satsIn] = spend

			satsIn += spend.Satoshis
			outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
			if spend.Listing {
				result.Events = append(result.Events, &Event{
					Channel: "unlist",
					Data:    outpoint[:],
				})
			}
			result.Spends = append(result.Spends, spend)
		}
	}

	var satsOut uint64
	for vout, txout := range tx.Outputs {
		if txout.Satoshis == 0 {
			continue
		}

		scripthash := sha256.Sum256(*txout.LockingScript)
		outpoint := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
		txo := &Txo{
			Txid:       txid,
			Vout:       uint32(vout),
			Satoshis:   txout.Satoshis,
			OutAcc:     satsOut,
			Scripthash: scripthash[:],
			Outpoint:   &outpoint,
		}

		if txout.Satoshis == 1 && (height == 0 || height >= uint32(783968)) {
			for _, spend := range result.Spends {
				if spend.Satoshis == 1 && spend.InAcc == txo.OutAcc {
					txo.Origin = spend.Origin
					txo.InOrd = spend
				}
			}
			parsed := ParseScript(*txout.LockingScript, tx, height)
			txo.Parsed = parsed
			txo.Lock = parsed.Lock

			if txo.Origin == nil && parsed.Inscription != nil && txo.Satoshis == 1 {
				txo.Origin = txo.Outpoint
			}

			txo.Listing = parsed.Listing != nil
			txo.Bsv20 = parsed.Bsv20 != nil && parsed.Bsv20.Op != "deploy"
			if parsed.Bsv20 != nil {
				result.Bsv20s = append(result.Bsv20s, parsed.Bsv20)
			}

			// result.ParsedScripts = append(result.ParsedScripts, parsed)
		}

		result.Txos = append(result.Txos, txo)
		satsOut += txout.Satoshis
	}

	if !dryRun {
		for _, txo := range result.Txos {
			// Implied BSV20 transfer
			if len(result.Bsv20s) == 0 && txo.InOrd != nil && txo.InOrd.Bsv20 {
				rows, err := db.Query(`SELECT tick, amt
					FROM bsv20_txos
					WHERE txid=$1 AND vout=$2 AND op != 'deploy'`,
					txo.InOrd.Txid,
					txo.InOrd.Vout,
				)
				if err != nil {
					log.Panic(err)
				}
				defer rows.Close()
				if rows.Next() {
					bsv20 := &Bsv20{
						Txid:    txo.Txid,
						Vout:    txo.Vout,
						Height:  height,
						Idx:     idx,
						Op:      "transfer",
						Lock:    txo.Lock,
						Implied: true,
					}

					err := rows.Scan(&bsv20.Ticker, &bsv20.Amt)
					if err != nil {
						log.Panic(err)
					}
					txo.Parsed.Bsv20 = bsv20
					txo.Bsv20 = true
				}
			}

			// Save Txo
			_, err = db.Exec(`INSERT INTO txos(txid, vout, height, idx, satoshis, outacc, scripthash, lock, listing, bsv20)
				VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT DO UPDATE SET 
					height=EXCLUDED.height,
					idx=EXCLUDED.idx,
					lock=EXCLUDED.lock,
					listing=EXCLUDED.listing,
					bsv20=EXCLUDED.bsv20`,
				txo.Txid,
				txo.Vout,
				txo.Height,
				txo.Vout,
				txo.Satoshis,
				txo.OutAcc,
				txo.Scripthash,
				txo.Lock,
				txo.Listing,
				txo.Bsv20,
			)
			if err != nil {
				log.Panicln("insTxo Err:", err)
			}

			if txo.Parsed != nil {
				if txo.Parsed.Inscription != nil {
					_, err = db.Exec(`INSERT INTO inscriptions(txid, vout, height, idx, origin, filehash, filesize, filetype, json_content, sigma)
						VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
						ON CONFLICT(txid, vout) DO UPDATE SET 
							height=EXCLUDED.height, 
							idx=EXCLUDED.idx, 
							origin=EXCLUDED.origin, 
							sigma=EXCLUDED.sigma`,
						txo.Txid,
						txo.Vout,
						txo.Height,
						txo.Idx,
						txo.Origin,
						txo.Parsed.Inscription.File.Hash,
						txo.Parsed.Inscription.File.Size,
						txo.Parsed.Inscription.File.Type,
						txo.Parsed.Inscription.JsonContent,
						txo.Parsed.Sigmas,
					)
					if err != nil {
						log.Panic(err)
					}
				}

				if txo.Parsed.Map != nil {
					_, err = db.Exec(`INSERT INTO map(txid, vout, height, idx, origin, map)
						VALUES($1, $2, $3, $4, $5, $6)
						ON CONFLICT DO UPDATE SET
							height=EXCLUDED.height,
							idx=EXCLUDED.idx,
							origin=EXCLUDED.origin,
							map=EXCLUDED.map`,
						txo.Txid,
						txo.Vout,
						txo.Height,
						txo.Idx,
						txo.Origin,
						txo.Parsed.Map,
					)
					if err != nil {
						log.Panic(err)
					}
					MAP := Map{}
					func() {
						rows, err := db.Query(`SELECT map FROM map 
							WHERE origin=$1 
							ORDER BY height, idx`,
							txo.Origin,
						)
						if err != nil {
							log.Panic(err)
						}
						defer rows.Close()
						for rows.Next() {
							var m Map
							err := rows.Scan(&m)
							if err != nil {
								log.Panic(err)
							}
							for k, v := range m {
								MAP[k] = v
							}
						}
					}()
					_, err = db.Exec(`UPDATE origins
						SET map=$2
						WHERE origin=$1`,
						txo.Origin,
						MAP,
					)
				}

				if txo.Parsed.Listing != nil {
					_, err = db.Exec(`INSERT INTO ordinal_lock_listings(txid, vout, height, idx, origin, price, payout, num, spend, lock, bsv20, map)
						SELECT $1, $2, $3, $4, $5, $6, $7, o.num, t.spend, t.lock, t.bsv20, o.map
						FROM txos t
						JOIN origins o ON o.origin = t.origin
						WHERE t.txid=$1 AND t.vout=$2
						ON CONFLICT(txid, vout) DO UPDATE
							SET height=EXCLUDED.height, 
								idx=EXCLUDED.idx, 
								origin=EXCLUDED.origin,
								lock=EXCLUDED.lock,
								bsv20=EXCLUDED.bsv20,
								map=EXCLUDED.map`,
						txo.Txid,
						txo.Vout,
						txo.Height,
						txo.Idx,
						txo.Origin,
						txo.Parsed.Listing.Price,
						txo.Parsed.Listing.PayOutput,
					)
					if err != nil {
						log.Panic(err)
					}
					result.Events = append(result.Events, &Event{
						Channel: "list",
						Data:    (*txo.Outpoint)[:],
					})
				}

				if txo.Parsed.Bsv20 != nil {
					b := txo.Parsed.Bsv20
					if b.Op == "deploy" {
						_, err := db.Exec(`INSERT INTO bsv20(txid, vout, id, height, idx, tick, max, lim, dec)
							VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
							ON CONFLICT(id) DO UPDATE SET
								height=EXCLUDED.height,
								idx=EXCLUDED.idx`,
							txo.Txid,
							txo.Vout,
							txo.Outpoint,
							height,
							idx,
							b.Ticker,
							b.Max,
							b.Limit,
							b.Decimals,
						)
						if err != nil {
							log.Panic(err)
						}
					}

					_, err := db.Exec(`INSERT INTO bsv20_txos(txid, vout, height, idx, tick, op, orig_amt, amt, lock, implied, spend, listing)
						SELECT $1, $2, $3, $4, $5, $6, $7, $7, lock, $8, spend, listing
						FROM txos
						WHERE txid=$1 AND vout=$2
						ON CONFLICT(txid, vout) DO UPDATE SET
							height=EXCLUDED.height,
							idx=EXCLUDED.idx`,
						b.Txid,
						b.Vout,
						b.Height,
						b.Idx,
						b.Ticker,
						b.Op,
						b.Amt,
						b.Implied,
					)
					if err != nil {
						log.Panic(err)
					}
				}
			}
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
