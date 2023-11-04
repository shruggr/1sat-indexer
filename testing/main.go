package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/GorillaPool/go-junglebus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var rdb *redis.Client
var bit *bitcoin.Bitcoind

var hexId = "f5684b29563ca4bfa9961153abea27438a7cd22133f559a18221ab4029377a45"

// var hexId = "58b7558ea379f24266c7e2f5fe321992ad9a724fd7a87423ba412677179ccb25"

func main() {
	godotenv.Load("../.env")
	var err error
	// re := regexp.MustCompile("\\W")
	// split := re.Split("1234.asdf the is the.thing", -1)
	// fmt.Println(split)
	db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}

	jb, err := junglebus.New(
		junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	)
	if err != nil {
		return
	}

	// port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	// bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	// if err != nil {
	// 	log.Panic(err)
	// }

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
	// 	// fmt.Printf("Completed: %d txns: %d\n", height, len(ctx.TxFees))
	// height := uint32(810142)
	// fmt.Println("Validating bsv20 for height", height)
	// lib.ValidateBsv20(height)
	// txid, _ := hex.DecodeString("c4e7018108e4529a63cdc8eef2256b786866868a5a66d4d6bb057589f344b3d1")
	// lib.ValidateTransfer(txid)

	// 	fmt.Println("Processing inscription ids for height", height)
	// 	err = lib.SetOriginNum(height)
	// 	if err != nil {
	// 		log.Panicln("Error processing inscription ids:", err)
	// 	}

	txn, err := jb.GetTransaction(context.Background(), hexId)
	if err != nil {
		log.Panic(err)
	}
	tx, err := bt.NewTxFromBytes(txn.Transaction)
	if err != nil {
		log.Panic(err)
	}

	// txid, _ := hex.DecodeString(hexId)
	// tx := bt.NewTx()
	// r, err := bit.GetRawTransactionRest(hexId)
	// if err != nil {
	// 	log.Panicf("%x: %v\n", txid, err)
	// }
	// if _, err = tx.ReadFrom(r); err != nil {
	// 	log.Panicf("%x: %v\n", txid, err)
	// }

	result, err := lib.IndexTxn(tx, nil, nil, 0, false)
	if err != nil {
		log.Panic(err)
	}

	out, err := json.MarshalIndent(result, "", "  ")

	if err != nil {
		log.Panic(err)
	}

	fmt.Println(string(out))
}
