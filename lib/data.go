package lib

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/GorillaPool/go-junglebus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
)

var TRIGGER = uint32(783968)

var Db *pgxpool.Pool
var Rdb *redis.Client
var JB *junglebus.Client
var bit *bitcoin.Bitcoind
var ctx = context.Background()

func Initialize(postgres *pgxpool.Pool, rdb *redis.Client) (err error) {
	Db = postgres
	Rdb = rdb

	log.Println("JUNGLEBUS", os.Getenv("JUNGLEBUS"))
	if os.Getenv("JUNGLEBUS") != "" {
		JB, err = junglebus.New(
			junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
		)
		if err != nil {
			return
		}
	}

	if os.Getenv("BITCOIN_HOST") != "" {
		port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
		bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
		if err != nil {
			log.Panic(err)
		}
	}

	return
}

func LoadTx(txid string) (tx *bt.Tx, err error) {
	rawtx, err := LoadRawtx(txid)
	if err != nil {
		return
	}
	if len(rawtx) == 0 {
		err = fmt.Errorf("missing-txn %s", txid)
		return
	}
	return bt.NewTxFromBytes(rawtx)
}

func LoadRawtx(txid string) (rawtx []byte, err error) {
	rawtx, _ = Rdb.HGet(ctx, "tx", txid).Bytes()

	if len(rawtx) > 0 {
		return rawtx, nil
	} else {
		rawtx = []byte{}

	}

	if len(rawtx) == 0 && JB != nil {
		if rawtx, err = JB.GetRawTransaction(ctx, txid); err != nil {
			log.Println("JB GetRawTransaction", err)
		}
	}

	if len(rawtx) == 0 && bit != nil {
		// log.Println("Requesting tx from node", txid)
		if r, err := bit.GetRawTransactionRest(txid); err == nil {
			rawtx, _ = io.ReadAll(r)
		}
	}

	if len(rawtx) == 0 {
		err = fmt.Errorf("LoadRawtx: missing-txn %s", txid)
		return
	}

	Rdb.HSet(ctx, "tx", txid, rawtx).Err()
	return
}

// func LoadTxOut(outpoint *Outpoint) (txout *bt.Output, err error) {
// 	txo, err := JB.GetTxo(ctx, hex.EncodeToString(outpoint.Txid()), outpoint.Vout())
// 	if err != nil {
// 		return
// 	}
// 	reader := bytes.NewReader(txo)
// 	txout = &bt.Output{}
// 	_, err = txout.ReadFrom(reader)
// 	return
// }

// func GetSpend(outpoint *Outpoint) (spend []byte, err error) {
// 	return JB.GetSpend(ctx, hex.EncodeToString(outpoint.Txid()), outpoint.Vout())
// }

// func GetChaintip() (*models.BlockHeader, error) {
// 	return JB.GetChaintip(ctx)

// }
