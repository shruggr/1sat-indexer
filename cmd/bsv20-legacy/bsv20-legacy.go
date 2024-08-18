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

func init() {
	// wd, _ := os.Getwd()
	// log.Println("CWD:", wd)
	godotenv.Load("../../.env")

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	var err error
	log.Println("POSTGRES:", POSTGRES)
	db, err := pgxpool.New(context.Background(), POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	cache := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISCACHE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb, cache)

	log.Println("JUNGLEBUS:", os.Getenv("JUNGLEBUS"))

}

func main() {
	rows, err := lib.Db.Query(context.Background(),
		`SELECT txid FROM bsv20_legacy`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	limiter := make(chan struct{}, 64)
	var wg sync.WaitGroup
	for rows.Next() {
		var txid []byte
		err := rows.Scan(&txid)
		if err != nil {
			log.Panicln(err)
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
				log.Printf("Err %x\n", txid)
				log.Panicln(err)
			}

			log.Printf("Processing %x\n", txid)
			ctx := ordinals.IndexTxn(tx.Transaction, tx.BlockHash, tx.BlockHeight, tx.BlockIndex)

			ordinals.IndexBsv20(ctx)
		}(txid)
	}
	wg.Wait()
}
