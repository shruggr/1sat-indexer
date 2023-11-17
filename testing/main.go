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
)

var rdb *redis.Client

var dryRun = false

var hexId = "47f94c875bc03f090f936c8723f7fb0685d465faafcb546243bd29ce3fe641ef"

func main() {
	godotenv.Load("../.env")
	var err error

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

	tx, err := lib.LoadTx(hexId)
	if err != nil {
		log.Panic(err)
	}

	result, err := lib.IndexTxn(tx, nil, nil, 0, dryRun)
	if err != nil {
		log.Panic(err)
	}

	out, err := json.MarshalIndent(result, "", "  ")

	if err != nil {
		log.Panic(err)
	}

	fmt.Println(string(out))
}
