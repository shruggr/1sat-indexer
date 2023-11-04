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

// var TxCache *lru.Cache[string, *bt.Tx]

var Db *pgxpool.Pool
var Rdb *redis.Client
var JB *junglebus.Client
var bit *bitcoin.Bitcoind

// var GetInput *sql.Stmt
// var GetMaxInscriptionNum *sql.Stmt
// var GetUnnumbered *sql.Stmt
// var InsTxo *sql.Stmt
// var InsBareSpend *sql.Stmt
// var InsSpend *sql.Stmt
// var InsInscription *sql.Stmt
// var InsMetadata *sql.Stmt
// var InsListing *sql.Stmt
// var SetSpend *sql.Stmt
// var SetInscriptionId *sql.Stmt

// var SetTxn *sql.Stmt

func Initialize(postgres *pgxpool.Pool, rdb *redis.Client) (err error) {
	// db = sdb
	Db = postgres
	Rdb = rdb

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
	rawtx, _ := Rdb.Get(context.Background(), txid).Bytes()

	if len(rawtx) > 0 {
		return bt.NewTxFromBytes(rawtx)
	}

	if JB != nil {
		if txData, err := JB.GetTransaction(context.Background(), txid); err == nil {
			rawtx = txData.Transaction
		}
	}

	if len(rawtx) == 0 && bit != nil {
		if r, err := bit.GetRawTransactionRest(txid); err == nil {
			rawtx, _ = io.ReadAll(r)
		}
	}

	if len(rawtx) == 0 {
		err = fmt.Errorf("missing-txn %s", txid)
		return
	}

	Rdb.Set(context.Background(), txid, rawtx, 0).Err()
	return bt.NewTxFromBytes(rawtx)
}

func LoadTxData(txid string) ([]byte, error) {
	txData, err := JB.GetTransaction(context.Background(), txid)
	if err != nil {
		return nil, err
	}
	return txData.Transaction, nil
}
