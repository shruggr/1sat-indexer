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

var hexId = "d578a8c71970c0643654b545516a717d1bafe9c6fc2957813c10688596eac5e6"

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
