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
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

var rdb *redis.Client

var dryRun = false

var hexId = "8531e165b8cdc5eae2d90853f36072f58fc92706e2673f77d7237cf4d744b09b"

func main() {
	godotenv.Load("../.env")
	var err error
	log.Println("POSTGRES_FULL:", os.Getenv("POSTGRES_FULL"))

	db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
	if err != nil {
		log.Panic(err)
	}
	defer db.Close()

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = ordinals.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordinals.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordlock.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	// ordinals.ValidateBsv20Mints(821874, "BSVS")
	rawtx, err := lib.LoadRawtx(hexId)
	if err != nil {
		log.Panic(err)
	}

	txnCtx := ordinals.IndexTxn(rawtx, "", 0, 0, dryRun)

	for _, txo := range txnCtx.Txos {
		// if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
		// 	fmt.Println(bsv20.Ticker)
		// 	bsv20.Save(txo)
		// }

		list := ordlock.ParseScript(txo)
		if list != nil {
			list.Save(txo)
		}
	}
	out, err := json.MarshalIndent(txnCtx, "", "  ")

	if err != nil {
		log.Panic(err)
	}

	fmt.Println(string(out))
}
