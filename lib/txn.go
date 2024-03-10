package lib

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"sync"

	"github.com/libsv/go-bt/v2"
)

const THREADS = 64

type IndexContext struct {
	Tx      *bt.Tx  `json:"-"`
	Txid    []byte  `json:"txid"`
	BlockId *string `json:"blockId"`
	Height  *uint32 `json:"height"`
	Idx     uint64  `json:"idx"`
	Txos    []*Txo  `json:"txos"`
	Spends  []*Txo  `json:"spends"`
}

func (ctx *IndexContext) Save() {
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

	limiter := make(chan struct{}, 32)
	var wg sync.WaitGroup
	for _, txo := range ctx.Txos {
		limiter <- struct{}{}
		wg.Add(1)
		go func(txo *Txo) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			txo.Save()
			if Rdb != nil {
				Rdb.Publish(context.Background(), hex.EncodeToString(txo.PKHash), txo.Outpoint.String())
			}
		}(txo)
	}
	wg.Wait()
}

func (ctx *IndexContext) SaveSpends() {
	limiter := make(chan struct{}, 32)
	var wg sync.WaitGroup

	for _, spend := range ctx.Spends {
		limiter <- struct{}{}
		wg.Add(1)
		go func(spend *Txo) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			spend.SaveSpend()
		}(spend)
	}
	wg.Wait()
}

func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64) (ctx *IndexContext, err error) {
	ctx, err = ParseTxn(rawtx, blockId, height, idx)
	ctx.SaveSpends()
	ctx.Save()
	return
}

func ParseTxn(rawtx []byte, blockId string, height uint32, idx uint64) (ctx *IndexContext, err error) {
	tx, err := bt.NewTxFromBytes(rawtx)
	if err != nil {
		panic(err)
	}
	txid := tx.TxIDBytes()
	ctx = &IndexContext{
		Tx:   tx,
		Txid: txid,
	}
	if height > 0 {
		ctx.BlockId = &blockId
		ctx.Height = &height
		ctx.Idx = idx
	}

	if !tx.IsCoinbase() {
		ParseSpends(ctx)
	}

	ParseTxos(tx, ctx)
	return
}

func ParseTxos(tx *bt.Tx, ctx *IndexContext) {
	accSats := uint64(0)
	for vout, txout := range tx.Outputs {
		outpoint := Outpoint(binary.BigEndian.AppendUint32(ctx.Txid, uint32(vout)))
		txo := &Txo{
			Tx:       tx,
			Height:   ctx.Height,
			Idx:      ctx.Idx,
			Satoshis: txout.Satoshis,
			OutAcc:   accSats,
			Outpoint: &outpoint,
			Script:   *txout.LockingScript,
		}

		if txout.LockingScript.IsP2PKH() {
			txo.PKHash = []byte((*txout.LockingScript)[3:23])
		}
		ctx.Txos = append(ctx.Txos, txo)
		accSats += txout.Satoshis
	}
}

func ParseSpends(ctx *IndexContext) {
	for vin, txin := range ctx.Tx.Inputs {
		spend := &Txo{
			Outpoint:    NewOutpoint(txin.PreviousTxID(), txin.PreviousTxOutIndex),
			Spend:       ctx.Txid,
			Vin:         uint32(vin),
			SpendHeight: ctx.Height,
			SpendIdx:    ctx.Idx,
		}

		ctx.Spends = append(ctx.Spends, spend)
	}
}

func LoadSpends(txid []byte, tx *bt.Tx) []*Txo {
	fmt.Println("Loading Spends", hex.EncodeToString(txid))
	var err error
	if tx == nil {
		tx, err = LoadTx(hex.EncodeToString(txid))
		if err != nil {
			log.Panic(err)
		}
	}

	spendByOutpoint := make(map[string]*Txo, len(tx.Inputs))
	spends := make([]*Txo, 0, len(tx.Inputs))

	rows, err := Db.Query(context.Background(), `
		SELECT outpoint, satoshis, outacc
		FROM txos
		WHERE spend=$1`,
		txid,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		spend := &Txo{
			Spend: txid,
		}
		var satoshis sql.NullInt64
		var outAcc sql.NullInt64
		err = rows.Scan(&spend.Outpoint, &satoshis, &outAcc)
		if err != nil {
			log.Panic(err)
		}
		if satoshis.Valid && outAcc.Valid {
			spend.Satoshis = uint64(satoshis.Int64)
			spend.OutAcc = uint64(outAcc.Int64)
			spendByOutpoint[spend.Outpoint.String()] = spend
		}
	}

	var inSats uint64
	for vin, txin := range tx.Inputs {
		outpoint := NewOutpoint(txin.PreviousTxID(), txin.PreviousTxOutIndex)
		spend, ok := spendByOutpoint[outpoint.String()]
		if !ok {
			spend = &Txo{
				Outpoint: outpoint,
				Spend:    txid,
				Vin:      uint32(vin),
			}

			tx, err := LoadTx(txin.PreviousTxIDStr())
			if err != nil {
				log.Panic(txin.PreviousTxIDStr(), err)
			}
			var outSats uint64
			for vout, txout := range tx.Outputs {
				if vout < int(spend.Outpoint.Vout()) {
					outSats += txout.Satoshis
					continue
				}
				spend.Satoshis = txout.Satoshis
				spend.OutAcc = outSats
				spend.Save()
				spendByOutpoint[outpoint.String()] = spend
				break
			}
		} else {
			spend.Vin = uint32(vin)
		}
		spends = append(spends, spend)
		inSats += spend.Satoshis
		// fmt.Println("Inputs:", spends[vin].Outpoint)
	}
	return spends
}
