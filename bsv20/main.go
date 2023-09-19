package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"
const TMP = "/tmp"

var THREADS uint64 = 128

var db *pgxpool.Pool

var rdb *redis.Client

func init() {
	godotenv.Load("../.env")

	var err error
	// port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	// bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	// if err != nil {
	// 	log.Panic(err)
	// }

	fmt.Println("BITCOIN", os.Getenv("BITCOIN_PORT"), os.Getenv("BITCOIN_HOST"))

	db, err = pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES"),
	)
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

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
		if err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	lib.ValidateBsv20(792554)
}
