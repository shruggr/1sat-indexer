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

var dryRun = true

var hexId = "366d667a03c5418688d4138a030d7dbf8b4a937afe0ca2072410d3545aa6c74e"

func main() {
	godotenv.Load("../.env")
	var err error
	db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
	if err != nil {
		log.Panic(err)
	}

	rdb := redis.NewClient(&redis.Options{
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
