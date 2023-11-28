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

var hexId = "b2a088b799a63265d72e2b5e3a6fb771ca3e8d743d24cb5c52bf6944698880ba"

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

	err = lib.Initialize(db, rdb)
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
