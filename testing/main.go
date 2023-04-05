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
	txid, _ := hex.DecodeString("1c6ce44ec1172e833a8ee27ac6a7a5209a0d35db1ebaabe01cf88cab9e1eb50c")
	txData, err := lib.LoadTxData(txid)
	if err != nil {
		log.Panic(err)
	}
	tx, err := bt.NewTxFromBytes(txData.Transaction)
	if err != nil {
		log.Panic(err)
	}
	result, err := lib.IndexTxn(tx, txData.BlockHeight, uint32(txData.BlockIndex), true)
	if err != nil {
		log.Panic(err)
	}
	out, err := json.Marshal(result)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(string(out))
}
