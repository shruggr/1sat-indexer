package lib

import (
	"bytes"
	"context"
	"time"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/bitcoin-sv/go-sdk/util"
)

const THREADS = 64

type Block struct {
	Height uint32 `json:"height"`
	Idx    uint64 `json:"idx"`
}

func ParseTxn(ctx context.Context, tx *transaction.Transaction, indexers []Indexer) (idxCtx *IndexContext, err error) {
	txidLE := tx.TxID().CloneBytes()
	txid := ByteString(util.ReverseBytes(txidLE))
	idxCtx = &IndexContext{
		Tx:   tx,
		Txid: &txid,
	}

	if tx.MerklePath != nil {
		idxCtx.Height = tx.MerklePath.BlockHeight
		for _, path := range tx.MerklePath.Path[0] {
			if bytes.Equal(txidLE, path.Hash.CloneBytes()) {
				idxCtx.Idx = path.Offset
				break
			}
		}
	} else {
		idxCtx.Height = uint32(time.Now().Unix())
	}

	if !tx.IsCoinbase() {
		idxCtx.ParseSpends(ctx)
	}

	ParseTxos(idxCtx, indexers)
	return
}

func ParseTxos(idxCtx *IndexContext, indexers []Indexer) {
	accSats := uint64(0)
	for vout, txout := range idxCtx.Tx.Outputs {
		outpoint := NewOutpoint(*idxCtx.Txid, uint32(vout))
		txo := &Txo{
			Height:   idxCtx.Height,
			Idx:      idxCtx.Idx,
			Satoshis: txout.Satoshis,
			OutAcc:   accSats,
			Outpoint: outpoint,
			Owners:   make(map[string]struct{}),
		}
		if len(*txout.LockingScript) >= 25 && script.NewFromBytes((*txout.LockingScript)[:25]).IsP2PKH() {
			pkhash := PKHash((*txout.LockingScript)[3:23])
			txo.Owners[pkhash.Address()] = struct{}{}
		}
		idxCtx.Txos = append(idxCtx.Txos, txo)
		accSats += txout.Satoshis
		for _, indexer := range indexers {
			if data := indexer.Parse(idxCtx, uint32(vout)); data != nil {
				txo.Data[indexer.Tag()] = data
			}
		}
	}
}

func (idxCtx *IndexContext) ParseSpends(ctx context.Context) {
	for _, txin := range idxCtx.Tx.Inputs {
		outpoint := NewOutpoint(util.ReverseBytes((*txin.SourceTXID)[:]), txin.SourceTxOutIndex)
		spend := &Txo{
			Outpoint:    outpoint,
			Spend:       *idxCtx.Txid,
			SpendHeight: idxCtx.Height,
			SpendIdx:    idxCtx.Idx,
		}

		idxCtx.Spends = append(idxCtx.Spends, spend)
	}
}

// var spendsCache = make(map[string][]*Txo)
// var m sync.Mutex

// func LoadSpends(txid []byte, tx *bt.Tx) []*Txo {
// 	// defer func() {
// 	// 	<-time.After(5 * time.Second)
// 	// 	m.Lock()
// 	// 	delete(spendsCache, hex.EncodeToString(txid))
// 	// 	m.Unlock()
// 	// }()
// 	// m.Lock()
// 	// sps, ok := spendsCache[hex.EncodeToString(txid)]
// 	// m.Unlock()
// 	// if ok {
// 	// 	return sps
// 	// }
// 	// fmt.Println("Loading Spends", hex.EncodeToString(txid))
// 	var err error
// 	if tx == nil {
// 		tx, err = LoadTx(hex.EncodeToString(txid))
// 		if err != nil {
// 			log.Panic(err)
// 		}
// 	}

// 	spendByOutpoint := make(map[string]*Txo, len(tx.Inputs))
// 	spends := make([]*Txo, 0, len(tx.Inputs))

// 	rows, err := Db.Query(context.Background(), `
// 		SELECT outpoint, satoshis, outacc
// 		FROM txos
// 		WHERE spend=$1`,
// 		txid,
// 	)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		spend := &Txo{
// 			Spend: txid,
// 		}
// 		var satoshis sql.NullInt64
// 		var outAcc sql.NullInt64
// 		err = rows.Scan(&spend.Outpoint, &satoshis, &outAcc)
// 		if err != nil {
// 			log.Panic(err)
// 		}
// 		if satoshis.Valid && outAcc.Valid {
// 			spend.Satoshis = uint64(satoshis.Int64)
// 			spend.OutAcc = uint64(outAcc.Int64)
// 			spendByOutpoint[spend.Outpoint.String()] = spend
// 		}
// 	}

// 	var inSats uint64
// 	for vin, txin := range tx.Inputs {
// 		outpoint := NewOutpoint(txin.PreviousTxID(), txin.PreviousTxOutIndex)
// 		spend, ok := spendByOutpoint[outpoint.String()]
// 		if !ok {
// 			spend = &Txo{
// 				Outpoint: outpoint,
// 				Spend:    txid,
// 				Vin:      uint32(vin),
// 			}

// 			tx, err := LoadTx(txin.PreviousTxIDStr())
// 			if err != nil {
// 				log.Panic(txin.PreviousTxIDStr(), err)
// 			}
// 			var outSats uint64
// 			for vout, txout := range tx.Outputs {
// 				if vout < int(spend.Outpoint.Vout()) {
// 					outSats += txout.Satoshis
// 					continue
// 				}
// 				spend.Satoshis = txout.Satoshis
// 				spend.OutAcc = outSats
// 				spend.Save()
// 				spendByOutpoint[outpoint.String()] = spend
// 				break
// 			}
// 		} else {
// 			spend.Vin = uint32(vin)
// 		}
// 		spends = append(spends, spend)
// 		inSats += spend.Satoshis
// 		// fmt.Println("Inputs:", spends[vin].Outpoint)
// 	}
// 	// m.Lock()
// 	// spendsCache[hex.EncodeToString(txid)] = spends
// 	// m.Unlock()
// 	return spends
// }
