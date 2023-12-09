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
)

var rdb *redis.Client

var dryRun = true

var hexId = "1bff350b55a113f7da23eaba1dc40a7c5b486d3e1017cda79dbe6bd42e001c81"

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

	rawtx, err := lib.LoadRawtx(hexId)
	if err != nil {
		log.Panic(err)
	}

	txnCtx := ordinals.IndexTxn(rawtx, "", 0, 0, dryRun)

	out, err := json.MarshalIndent(txnCtx, "", "  ")

	if err != nil {
		log.Panic(err)
	}

	fmt.Println(string(out))
}
