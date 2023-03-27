package lib

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"log"

	"github.com/libsv/go-bt/v2"
	"github.com/tikv/client-go/v2/txnkv/transaction"
)

type Txn struct {
	Txid        []byte `json:"txid"`
	BlockHeight uint32 `json:"height"`
	BlockIndex  uint32 `json:"idx"`
	Fee         uint64 `json:"fee"`
	AccFees     uint64 `json:"acc_fees"`
	Inputs	  []*Txo `json:"inputs"`
	Outputs	 []*Txo `json:"outputs"`
}

func (t *Txn) KVKey() []byte {
	buf := bytes.NewBuffer([]byte{'t'})
	binary.Write(buf, binary.BigEndian, t.Txid)
	return buf.Bytes()
}

func (t *Txn) KVValue() []byte {
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, t.BlockHeight)
	binary.Write(buf, binary.BigEndian, t.BlockIndex)
	return buf.Bytes()
}

func (t *Txn) BlockKVKey() []byte {
	buf := bytes.NewBuffer([]byte{'b'})
	binary.Write(buf, binary.BigEndian, t.BlockHeight)
	binary.Write(buf, binary.BigEndian, t.AccFees)
	return buf.Bytes()
}

func (t *Txn) BlockKVValue() []byte {
	buf := bytes.NewBuffer(t.Txid[:32])
	binary.Write(buf, binary.BigEndian, t.Fee)
	return buf.Bytes()
}

type IndexResult struct {
	Txos   []*Txo             `json:"txos"`
	Ims    []*InscriptionMeta `json:"ims"`
	Spends []*Spend           `json:"spends"`
}

func IndexTx(tx *bt.Tx, blockHeight uint32, blockIndex uint32, save bool) (, err error) {

}

func IndexSpends(tx *bt.Tx, save bool) (spends []*Spend, err error) {
	txid := tx.TxIDBytes()
	var accSats uint64
	for vin, txin := range tx.Inputs {
		txo := &Txo{
			Txid: txin.PreviousTxID(),
			Vout: txin.PreviousTxOutIndex,
		}
		var txn *transaction.KVTxn
		txn, err = Tikv.Begin()
		if err != nil {
			return
		}
		defer txn.Rollback()
		var txoData []byte
		txoData, err = txn.Get(context.TODO(), txo.OutputKey())
		if err != nil {
			return
		}
		err = txo.OutputPopulate(txoData)
		if err != nil {
			return
		}
		txo.SpendTxid = txid
		txo.SpendVin = uint32(vin)
		accSats += txo.Satoshis

		spend := &Spend{
			Txid:    txid,
			Vin:     uint32(vin),
			InTxid:  txin.PreviousTxID(),
			InVout:  txin.PreviousTxOutIndex,
			AccSats: accSats,
		}
		spends = append(spends, spend)

		if save {
			err = Tikv.Delete(context.TODO(), txo.UtxoKVKey())
			if err != nil {
				return
			}
			err = Tikv.Put(context.TODO(), txo.KVKey(), txo.KVValue())
			if err != nil {
				return
			}
			err = Tikv.Put(context.TODO(), spend.KVKey(), spend.KVValue())
			if err != nil {
				return
			}
		}
	}
	return
}

func IndexTxos(tx *bt.Tx, height uint32, idx uint32, save bool) (txos []*Txo, err error) {
	txid := tx.TxIDBytes()
	var accSats uint64
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

		hash := sha256.Sum256(*txout.LockingScript)
		txo.ScriptHash = bt.ReverseBytes(hash[:])
		txos = append(txos, txo)
		if save {
			err = Tikv.Put(context.TODO(), txo.KVKey(), txo.KVValue())
			if err != nil {
				return
			}
			err = Tikv.Put(context.TODO(), txo.UtxoKVKey(), txo.UtxoKVValue())
			if err != nil {
				return
			}
		}
	}
	return
}

func Index1SatSpends(tx *bt.Tx, save bool) (spends []*Spend, err error) {
	txid := tx.TxIDBytes()
	for vin, txin := range tx.Inputs {
		spend := &Spend{
			InTxid: txin.PreviousTxID(),
			InVout: txin.PreviousTxOutIndex,
			Txid:   txid,
			Vin:    uint32(vin),
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

func IndexInscriptionTxos(tx *bt.Tx, height uint32, idx uint32, save bool) (result *IndexResult, err error) {
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
		txo.Origin, err = LoadOrigin(txo)
		if err != nil {
			return
		}
		var im *InscriptionMeta
		im, err = ParseInscriptionOutput(txout)
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
			&inTxo.SpendTxid,
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
	} else {
		origin = binary.BigEndian.AppendUint32(txo.Txid, txo.Vout)
		rows.Close()
	}

	return origin, nil
}
