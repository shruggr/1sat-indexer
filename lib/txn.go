package lib

import (
	"crypto/sha256"
	"encoding/binary"
	"log"

	"github.com/libsv/go-bt/v2"
)

type Status uint

var (
	Unconfirmed Status = 0
	Confirmed   Status = 1
)

type IndexResult struct {
	Txos   []*Txo             `json:"txos"`
	Ims    []*InscriptionMeta `json:"ims"`
	Spends []*Spend           `json:"spends"`
}

func IndexSpends(tx *bt.Tx, save bool) (spends []*Spend, err error) {
	txid := tx.TxIDBytes()
	for vin, txin := range tx.Inputs {
		spend := &Spend{
			Txid:  txin.PreviousTxID(),
			Vout:  txin.PreviousTxOutIndex,
			Spend: txid,
			Vin:   uint32(vin),
		}
		spends = append(spends, spend)
		if save {
			err = spend.Save()
			if err != nil {
				return
			}
		}
	}
	return
}

func IndexTxos(tx *bt.Tx, height uint32, idx uint32, save bool) (result *IndexResult, err error) {
	txid := tx.TxIDBytes()
	result = &IndexResult{
		Txos: []*Txo{},
		Ims:  []*InscriptionMeta{},
	}
	var accSats uint64
	for vout, txout := range tx.Outputs {
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
		if height > 0 {
			txo.Origin, err = LoadOrigin(txo)
			if err != nil {
				return
			}
		}
		var im *InscriptionMeta
		im, err = ParseOutput(txout)
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
			if save {
				err = im.Save()
				if err != nil {
					return
				}
			}
			result.Ims = append(result.Ims, im)
			if err != nil {
				return
			}
		} else {
			hash := sha256.Sum256(*txout.LockingScript)
			txo.Lock = bt.ReverseBytes(hash[:])
		}
		result.Txos = append(result.Txos, txo)
		if save {
			err = txo.Save()
			if err != nil {
				return
			}
		}
	}
	return
}

func ParseOutput(txout *bt.Output) (im *InscriptionMeta, err error) {
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
		if len(inTxo.Origin) > 0 {
			origin = inTxo.Origin
			return
		}
	} else {
		origin = binary.BigEndian.AppendUint32(txo.Txid, txo.Vout)
	}

	return origin, nil
}
