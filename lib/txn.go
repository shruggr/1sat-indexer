package lib

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"log"

	"github.com/libsv/go-bt/v2"
)

type Status uint

var (
	Unconfirmed Status = 0
	Confirmed   Status = 1
)

type IndexResult struct {
	Txos          []*Txo          `json:"txos"`
	ParsedScripts []*ParsedScript `json:"parsed"`
	Spends        []*Txo          `json:"spends"`
}

func IndexTxn(tx *bt.Tx, height uint32, idx uint32, save bool) (result *IndexResult, err error) {
	result = &IndexResult{}
	txid := tx.TxIDBytes()
	var accSats uint64
	for vin, txin := range tx.Inputs {
		spend := &Txo{
			Txid:  txin.PreviousTxID(),
			Vout:  txin.PreviousTxOutIndex,
			Spend: txid,
			Vin:   uint32(vin),
		}
		result.Spends = append(result.Spends, spend)
		if save {
			var exists bool
			exists, err = spend.SaveSpend()
			if err != nil {
				log.Panic(err)
				return
			}
			if !exists && save {
				var tx *bt.Tx
				tx, err = LoadTx(spend.Txid)
				if err != nil {
					log.Panic(err)
					return
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
					return
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
		}
	}

	accSats = 0
	for vout, txout := range tx.Outputs {
		accSats += txout.Satoshis
		outpoint := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
		msg := outpoint.String()
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
			}
		}

		parsed := ParseScript(*txout.LockingScript, true)
		if txo.Origin == nil && parsed.Ord != nil {
			txo.Origin = &outpoint
		}
		if txo.Origin != nil {
			txo.Lock = parsed.Lock

			parsed.Txid = txid
			parsed.Vout = uint32(vout)
			parsed.Height = height
			parsed.Idx = idx
			parsed.Origin = txo.Origin
			if save {
				if txo.Origin == &outpoint {
					err = parsed.SaveInscription()
					if err != nil {
						return
					}
				}
				err = parsed.Save()
				if err != nil {
					return
				}
			}

			if len(parsed.Listings) > 0 {
				if err != nil {
					return
				}
				for _, l := range parsed.Listings {
					l.Txid = txid
					l.Vout = uint32(vout)
					l.Origin = txo.Origin
					l.Height = height
					l.Idx = idx
					txo.Listing = true
					if save {
						err = l.Save()
						if err != nil {
							return
						}
						Rdb.Publish(context.Background(), "list", msg)
					}
				}
			}
			result.ParsedScripts = append(result.ParsedScripts, parsed)
		}
		result.Txos = append(result.Txos, txo)
		if save {
			err = txo.Save()
			if err != nil {
				return
			}
			Rdb.Publish(context.Background(), hex.EncodeToString(txo.Lock), msg)
		}
	}
	return
}

// func LoadOrigin(txo *Txo) (origin *Outpoint, err error) {
// 	if txo.Satoshis != 1 {
// 		return
// 	}
// 	fmt.Printf("load origin %x %d\n", txo.Txid, txo.AccSats)
// 	rows, err := GetInput.Query(txo.Txid, txo.AccSats)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()
// 	if rows.Next() {
// 		inTxo := &Txo{}
// 		err = rows.Scan(
// 			&inTxo.Txid,
// 			&inTxo.Vout,
// 			&inTxo.Satoshis,
// 			&inTxo.AccSats,
// 			&inTxo.Lock,
// 			&inTxo.Spend,
// 			&inTxo.Origin,
// 		)
// 		if err != nil {
// 			return
// 		}
// 		if inTxo.Origin != nil {
// 			origin = inTxo.Origin
// 			return
// 		}
// 	}

// 	return origin, nil
// }
