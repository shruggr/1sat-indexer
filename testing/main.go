package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var rdb *redis.Client

func main() {
	godotenv.Load("../.env")

	db, err := sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
	txid, _ := hex.DecodeString("ce47c96d10592c6e90a9b6dc540d842f8817af929f7161b4b3c4e88583c2b6d5")
	txData, err := lib.LoadTxData(txid)
	if err != nil {
		log.Panic(err)
	}
	tx, err := bt.NewTxFromBytes(txData.Transaction)
	if err != nil {
		log.Panic(err)
	}
	result, err := lib.IndexTxn(tx, txData.BlockHeight, uint32(txData.BlockIndex), false)
	if err != nil {
		log.Panic(err)
	}
	out, err := json.Marshal(result)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(string(out))
}
