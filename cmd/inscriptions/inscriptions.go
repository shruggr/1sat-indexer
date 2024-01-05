package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client
var INDEXER string
var TOPIC string
var FROM_BLOCK uint
var VERBOSE int
var CONCURRENCY int = 64

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	// flag.StringVar(&INDEXER, "id", "", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

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

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	blockDone := make(chan uint32, 10000)

	go func() {
		for blockHeight := range blockDone {
			row := db.QueryRow(context.Background(),
				"SELECT MAX(num) FROM inscriptions",
			)
			var num int
			err := row.Scan(&num)
			if err != nil {
				if err == pgx.ErrNoRows {
					return
				}
				panic(err)
			}
			log.Println("Start height", blockHeight-6)
			rows, err := db.Query(context.Background(), `
				SELECT height, idx, vout
				FROM inscriptions
				WHERE num=-1 AND height<$1
				ORDER BY height, idx, vout`,
				blockHeight-6,
			)
			if err != nil {
				panic(err)
			}
			for rows.Next() {
				var height uint32
				var idx uint64
				var vout uint32
				err = rows.Scan(&height, &idx, &vout)
				if err != nil {
					panic(err)
				}

				num++
				_, err = db.Exec(context.Background(), `
					UPDATE inscriptions
					SET num=$1
					WHERE height=$2 AND idx=$3 AND vout=$4`,
					num,
					height,
					idx,
					vout,
				)
				if err != nil {
					panic(err)
				}
			}
			fmt.Println("Inscription num:", num)
		}
	}()

	err := indexer.Exec(
		true,
		false,
		func(ctx *lib.IndexContext) error {
			ordinals.ParseInscriptions(ctx)
			for vout, txo := range ctx.Txos {
				if _, ok := txo.Data["insc"]; !ok {
					continue
				}
				_, err := db.Exec(context.Background(), `
					INSERT INTO inscriptions(height, idx, vout)
					VALUES($1, $2, $3)
					ON CONFLICT DO NOTHING`,
					ctx.Height,
					ctx.Idx,
					vout,
				)
				if err != nil {
					return err
				}
			}
			return nil
		},
		func(height uint32) error {
			blockDone <- height
			return nil
		},
		"inscriptions",
		TOPIC,
		FROM_BLOCK,
		CONCURRENCY,
		false,
		false,
		VERBOSE,
	)
	if err != nil {
		log.Panicln(err)
	}
}
