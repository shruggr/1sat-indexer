package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

// var settled = make(chan uint32, 1000)
var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client
var INDEXER = "bsv20-initial"
var TOPIC string
var VERBOSE int
var CONCURRENCY int = 64

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	// flag.StringVar(&INDEXER, "id", "", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	// flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	// flag.IntVar(&VERBOSE, "v", 0, "Verbose")
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

	err = ordinals.Initialize(indexer.Db, indexer.Rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	var progress uint
	row := db.QueryRow(context.Background(), `SELECT height
			FROM progress
			WHERE indexer=$1`,
		INDEXER,
	)
	row.Scan(&progress)
	if progress > 807000 {
		fmt.Println("Done with initial sync")
		time.Sleep(24 * time.Hour)
		return
	}
	err := indexer.Exec(
		true,
		false,
		func(tx *lib.IndexContext) error {
			ordinals.ParseInscriptions(tx)
			for _, txo := range tx.Txos {
				if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
					if bsv20.Ticker == "" {
						continue
					}
					list := ordlock.ParseScript(txo)

					if list != nil {
						txo.PKHash = list.PKHash
						bsv20.PKHash = list.PKHash
						bsv20.Price = list.Price
						bsv20.PayOut = list.PayOut
						bsv20.Listing = true
						var token *ordinals.Bsv20
						if bsv20.Id != nil {
							token = ordinals.LoadTokenById(bsv20.Id)
						} else if bsv20.Ticker != "" {
							token = ordinals.LoadTicker(bsv20.Ticker)
						}
						var decimals uint8
						if token != nil {
							decimals = token.Decimals
						}
						bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt*(10^uint64(decimals)))
					}
					bsv20.Save(txo)
				}
			}
			return nil
		},
		func(height uint32) error {
			if height > 807000 {
				log.Panicln("Done with initial sync")
			}
			return nil
		},
		INDEXER,
		TOPIC,
		uint(lib.TRIGGER),
		CONCURRENCY,
		true,
		true,
		VERBOSE,
	)
	if err != nil {
		log.Println("Subscription Error:", TOPIC, err)
	}

}

// func handleBlock(height uint32) error {

// 	return nil
// }
