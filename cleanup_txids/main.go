package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/GorillaPool/go-junglebus"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var THREADS uint64 = 32

var db *pgxpool.Pool
var junglebusClient *junglebus.Client

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES"),
	)

	rdb := redis.NewClient(&redis.Options{
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
	fmt.Println("Starting Ord", os.Getenv("JUNGLEBUS"))
	var err error
	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

	rows, err := db.Query(context.Background(), `
		SELECT txid 
		FROM txns
		WHERE created < now() - interval '12h' AND height IS NULL`,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	var wg sync.WaitGroup
	limiter := make(chan struct{}, THREADS)
	for rows.Next() {
		var txid []byte
		err = rows.Scan(&txid)
		if err != nil {
			log.Panic(err)
		}

		wg.Add(1)
		limiter <- struct{}{}
		go func(txid []byte) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			tx, err := junglebusClient.GetTransaction(context.Background(), hex.EncodeToString(txid))
			if err != nil {
				log.Panic(err)
			}
			if tx.BlockHeight > 0 {
				fmt.Printf("%x %d %d\n", txid, tx.BlockHeight, tx.BlockIndex)
				t, err := db.Begin(context.Background())
				if err != nil {
					log.Panic(err)
				}
				defer t.Rollback(context.Background())
				_, err = t.Exec(context.Background(), `
					UPDATE txns
					SET height=$2, idx=$3
					WHERE txid=$1`,
					txid,
					tx.BlockHeight,
					tx.BlockIndex,
				)
				if err != nil {
					log.Panic(err)
				}
				_, err = t.Exec(context.Background(), `
					UPDATE txos
					SET height=$2, idx=$3
					WHERE txid=$1`,
					txid,
					tx.BlockHeight,
					tx.BlockIndex,
				)
				if err != nil {
					log.Panic(err)
				}
				err = t.Commit(context.Background())
				if err != nil {
					log.Panic(err)
				}
			}
		}(txid)
	}

	wg.Wait()
}
