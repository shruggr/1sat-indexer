package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

// var settled = make(chan uint32, 1000)
var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client
var VERBOSE int
var CONCURRENCY int
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	var err error
	log.Println("POSTGRES:", POSTGRES)
	db, err = pgxpool.New(ctx, POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
}

func main() {
	rows, err := db.Query(ctx, `
		SELECT id, txid, spend, height, idx
		FROM bsv20_txos
		WHERE id != '\x'
		ORDER BY height, id`,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	counter := 0
	for rows.Next() {
		var id lib.Outpoint
		var txid []byte
		var spend []byte
		var height sql.NullInt32
		var idx sql.NullInt64
		err = rows.Scan(&id, &txid, &spend, &height, &idx)
		if err != nil {
			log.Panic(err)
		}
		counter++
		if len(spend) == 0 {
			continue
		}
		var score float64
		if height.Int32 == 0 {
			score = 0x1FFFFF + float64(time.Now().Unix())*math.Pow(2, -31)
		} else {
			score = float64(height.Int32) + float64(idx.Int64)*math.Pow(2, -31)
		}

		log.Printf("Saving Dep: %d %x %x\n", counter, spend, txid)
		if rdb.ZAdd(ctx, fmt.Sprintf("dep:%x", spend), redis.Z{
			Member: hex.EncodeToString(txid),
			Score:  score,
		}).Err(); err != nil {
			log.Panic(err)
		}
	}
}
