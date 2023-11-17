package lib

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"log"

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
}

func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64, dryRun bool) (ctx *IndexContext, err error) {
	tx, err := bt.NewTxFromBytes(rawtx)
	if err != nil {
		return
	}
	txid := tx.TxIDBytes()
	ctx = &IndexContext{
		Tx:   tx,
		Txid: txid,
	}
	if blockId != "" {
		ctx.BlockId = &blockId
		ctx.Height = &height
		ctx.Idx = idx
	}

	if !tx.IsCoinbase() {
		SetSpends(tx)
	}

	IndexTxos(tx, ctx, dryRun)

	return
}

func IndexTxos(tx *bt.Tx, ctx *IndexContext, dryRun bool) {
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
		}

		ParseScript(txo)
		if txo.Satoshis == 1 {
			txo.Origin = LoadOrigin(txo.Outpoint, txo.OutAcc)
		}
		ctx.Txos = append(ctx.Txos, txo)
		accSats += txout.Satoshis
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
			if Rdb != nil {
				Rdb.Publish(context.Background(), hex.EncodeToString(txo.PKHash), txo.Outpoint.String())
			}
			txo.Save()

			if txo.Origin == nil {
				continue
			}
			if bytes.Equal(*txo.Origin, *txo.Outpoint) {
				origin := &Origin{
					Origin: txo.Outpoint,
					Height: ctx.Height,
					Idx:    ctx.Idx,
				}
				if txo.Data != nil && txo.Data.Map != nil {
					origin.Map = txo.Data.Map
				}
				origin.Save()
			} else if txo.Data != nil && txo.Data.Map != nil {
				SaveMap(txo.Origin)
			}
		}
	}
}

func SetSpends(tx *bt.Tx) {
	// outpoints := make([][]byte, len(tx.Inputs))
	spend := tx.TxIDBytes()
	for vin, txin := range tx.Inputs {
		outpoint := NewOutpoint(txin.PreviousTxID(), txin.PreviousTxOutIndex)
		// outpoints[vin] = *outpoint
		if _, err := Db.Exec(context.Background(),
			"UPDATE txos SET spend=$2, vin=$3 WHERE outpoint=$1",
			outpoint,
			spend,
			vin,
		); err != nil {
			log.Panic(err)
		}
	}
}

func LoadSpends(txid []byte, tx *bt.Tx) []*Txo {
	// fmt.Println("Loading Spends", hex.EncodeToString(txid))
	var err error
	if tx == nil {
		tx, err = LoadTx(hex.EncodeToString(txid))
		if err != nil {
			log.Panic(err)
		}
	}

	spends := make([]*Txo, len(tx.Inputs))

	rows, err := Db.Query(context.Background(), `
		SELECT outpoint, vin, satoshis, outacc
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
		err = rows.Scan(&spend.Outpoint, &spend.Vin, &satoshis, &outAcc)
		if err != nil {
			log.Panic(err)
		}
		if satoshis.Valid && outAcc.Valid {
			spend.Satoshis = uint64(satoshis.Int64)
			spend.OutAcc = uint64(outAcc.Int64)
			spends[spend.Vin] = spend
		}
	}

	var inSats uint64
	for vin, txin := range tx.Inputs {
		if spends[vin] == nil {
			spend := &Txo{
				Outpoint: NewOutpoint(txin.PreviousTxID(), txin.PreviousTxOutIndex),
				Spend:    txid,
				Vin:      uint32(vin),
			}

			tx, err := LoadTx(txin.PreviousTxIDStr())
			if err != nil {
				log.Panic(err)
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
				spends[vin] = spend
				break
			}
		}
		inSats += spends[vin].Satoshis
		// fmt.Println("Inputs:", spends[vin].Outpoint)
	}
	return spends
}
