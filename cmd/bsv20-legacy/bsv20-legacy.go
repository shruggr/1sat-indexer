package main

import (
	"context"
	"encoding/hex"
	"log"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client

func init() {
	// wd, _ := os.Getwd()
	// log.Println("CWD:", wd)
	godotenv.Load("../../.env")

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

	log.Println("JUNGLEBUS:", os.Getenv("JUNGLEBUS"))

	err = ordinals.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	rows, err := db.Query(context.Background(),
		`SELECT txid FROM bsv20_legacy`,
	)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	limiter := make(chan struct{}, 64)
	var wg sync.WaitGroup
	for rows.Next() {
		var txid []byte
		err := rows.Scan(&txid)
		if err != nil {
			panic(err)
		}
		limiter <- struct{}{}
		wg.Add(1)
		go func(txid []byte) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			tx, err := lib.JB.GetTransaction(context.Background(), hex.EncodeToString(txid))
			if err != nil {
				panic(err)
			}

			log.Printf("Processing %x\n", txid)
			ctx := ordinals.IndexTxn(tx.Transaction, tx.BlockHash, tx.BlockHeight, tx.BlockIndex)

			for _, txo := range ctx.Txos {
				if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
					bsv20.Save(txo)
				}
			}
		}(txid)
	}
	wg.Wait()
}
