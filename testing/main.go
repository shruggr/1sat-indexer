package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/opns"
)

var rdb *redis.Client

var dryRun = false

var hexId = "2a7613a5c5fc212d23c3cdcbd9a5941c7d3bc6f1bf87eedac7200e314bf78ca9"

func main() {
	godotenv.Load("../.env")
	var err error
	log.Println("POSTGRES_FULL:", os.Getenv("POSTGRES_FULL"))

	db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
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

	rawtx, err := lib.LoadRawtx(hexId)
	if err != nil {
		log.Panic(err)
	}

	txnCtx := opns.IndexTxn(rawtx, "", 0, 0, dryRun)

	out, err := json.MarshalIndent(txnCtx, "", "  ")

	if err != nil {
		log.Panic(err)
	}

	fmt.Println(string(out))
}
