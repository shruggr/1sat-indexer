package lib

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/libsv/go-bt/v2"
)

func IndexTxos(tx *bt.Tx, height uint32, idx uint32) (err error) {
	txid := tx.TxIDBytes()
	// fmt.Printf("Indexing Txos %x\n", txid)

	for vout, txout := range tx.Outputs {
		var accSats uint64
		accSats += txout.Satoshis
		if txout.Satoshis != 1 {
			continue
		}
		txo := &Txo{
			Txid:     txid,
			Vout:     uint32(vout),
			Height:   height,
			Idx:      idx,
			Satoshis: txout.Satoshis,
			AccSats:  accSats,
		}
		txo.Origin, err = LoadOrigin(txo)
		if err != nil {
			return
		}
		var im *InscriptionMeta
		im, err = ProcessInsOutput(txout)
		if err != nil {
			log.Println("ProcessOutput Err:", err)
			return
		}
		if im != nil {
			im.Txid = txid
			im.Vout = uint32(vout)
			im.Height = height
			im.Idx = idx
			im.Origin = txo.Origin
			txo.Lock = im.Lock
			err = im.Save()
			if err != nil {
				return
			}
		} else {
			hash := sha256.Sum256(*txout.LockingScript)
			txo.Lock = bt.ReverseBytes(hash[:])
		}

		_, err = InsTxo.Exec(
			txo.Txid,
			txo.Vout,
			txo.Satoshis,
			txo.AccSats,
			txo.Lock,
			txo.Origin,
			txo.Height,
			txo.Idx,
		)
		if err != nil {
			log.Println("insTxo Err:", err)
			return
		}
	}

	for vin, txin := range tx.Inputs {
		_, err = SetSpend.Exec(
			txin.PreviousTxID(),
			txin.PreviousTxOutIndex,
			txid,
			vin,
		)
		if err != nil {
			return
		}
	}

	fmt.Print(".")
	return
}

// func IndexOrigins(txid []byte) (err error) {
// 	fmt.Printf("Indexing Origins %x\n", txid)
// 	txos, err := LoadTxos(txid)
// 	for _, txo := range txos {
// 		// var origin []byte
// 		_, err = LoadOrigin(txo)
// 		if err != nil {
// 			log.Println("LoadOrigin Err:", err)
// 			return
// 		}
// 	}

// 	// fmt.Printf("Done %x\n", txid)
// 	return
// }

func ProcessInsOutput(txout *bt.Output) (im *InscriptionMeta, err error) {
	inscription, lock := InscriptionFromScript(*txout.LockingScript)
	if inscription == nil {
		return
	}

	hash := sha256.Sum256(inscription.Body)
	im = &InscriptionMeta{
		File: File{
			Hash: hash[:],
			Size: uint32(len(inscription.Body)),
			Type: inscription.Type,
		},
		Lock: lock[:],
	}
	return
}

func LoadOrigin(txo *Txo) (origin Origin, err error) {
	rows, err := GetInput.Query(txo.Txid, txo.AccSats)
	if err != nil {
		return
	}

	defer rows.Close()
	if rows.Next() {
		inTxo := &Txo{}
		err = rows.Scan(
			&inTxo.Txid,
			&inTxo.Vout,
			&inTxo.Satoshis,
			&inTxo.AccSats,
			&inTxo.Lock,
			&inTxo.Spend,
			&inTxo.Origin,
		)
		if err != nil {
			return
		}
		rows.Close()
		if len(inTxo.Origin) > 0 {
			origin = inTxo.Origin
			return
		}
		// origin, err = LoadOrigin(inTxo)
		// if err != nil {
		// 	return
		// }
	} else {
		origin = binary.BigEndian.AppendUint32(txo.Txid, txo.Vout)
		rows.Close()
	}

	// if len(origin) > 0 {
	// 	_, err = SetTxoOrigin.Exec(
	// 		txo.Txid,
	// 		txo.Vout,
	// 		origin,
	// 	)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	_, err = SetInscriptionOrigin.Exec(
	// 		txo.Txid,
	// 		txo.Vout,
	// 		origin,
	// 	)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	return origin, nil
}
