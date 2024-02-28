package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

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

	flag.StringVar(&INDEXER, "id", "", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
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
		Addr:     os.Getenv("REDIS"),
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
	err := indexer.Exec(
		true,
		true,
		handleTx,
		handleBlock,
		INDEXER,
		TOPIC,
		FROM_BLOCK,
		CONCURRENCY,
		true,
		true,
		VERBOSE,
	)
	if err != nil {
		log.Panicln(err)
	}
}

func handleTx(tx *lib.IndexContext) error {
	ordinals.ParseInscriptions(tx)
	ordinals.CalculateOrigins(tx)
	xfers := map[string]*ordinals.Bsv20{}
	for _, txo := range tx.Txos {
		if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok && strings.HasPrefix(bsv20.Op, "deploy") {
			bsv20.Save(txo)
			// if mempool tx, que for immediate validation
			// if mined tx, skip and it will be validated by block handler
			if tx.Height == nil && bsv20.Op == "transfer" {
				if bsv20.Ticker != "" {
					xfers[bsv20.Ticker] = bsv20
				} else {
					xfers[bsv20.Id.String()] = bsv20
				}
			}
		}
	}
	for _, bsv20 := range xfers {
		if bsv20.Ticker != "" {
			ordinals.ValidateV1Transfer(tx.Txid, bsv20.Ticker, false)
		} else {
			ordinals.ValidateV2Transfer(tx.Txid, bsv20.Id, false)
		}
	}
	return nil
}

func handleBlock(height uint32) error {
	ordinals.ValidateBsv20Deploy(height - 6)
	ordinals.ValidateBsv20Txos(height - 6)
	return nil
}
