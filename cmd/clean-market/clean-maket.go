package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client

var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	var err error
	log.Println("POSTGRES:", POSTGRES)
	db, err = pgxpool.New(context.Background(), POSTGRES)
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
}

func main() {
	rows, err := db.Query(ctx, `SELECT txid, vout 
		FROM listings
		WHERE spend = '\x'`)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()

	limiter := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for rows.Next() {
		var txid []byte
		var vout uint
		err := rows.Scan(&txid, &vout)
		if err != nil {
			log.Panicln(err)
		}

		wg.Add(1)
		limiter <- struct{}{}
		go func(txid []byte, vout uint) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			url := fmt.Sprintf("%s/v1/txo/spend/%x_%d", os.Getenv("JUNGLEBUS"), txid, vout)
			// log.Println("URL:", url)
			resp, err := http.Get(url)
			if err != nil {
				log.Panicln(err)
			}
			spend, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Panicln(err)
			}
			if len(spend) == 0 {
				log.Printf("Unpent: %x_%d\n", txid, vout)
				return
			}
			log.Printf("Spent: %x_%d\n", txid, vout)
			rawtx, err := lib.LoadRawtx(hex.EncodeToString(spend))
			if err != nil {
				log.Panicln(err)
			}
			lib.IndexTxn(rawtx, "", 0, 0)
		}(txid, vout)
	}
	wg.Wait()
}
