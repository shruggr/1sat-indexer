package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
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
	// log.Println("POSTGRES:", POSTGRES)
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

	err = ordinals.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordlock.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	err := indexer.Exec(
		true,
		false,
		handleTx,
		func(height uint32) error {
			return nil
		},
		INDEXER,
		TOPIC,
		FROM_BLOCK,
		CONCURRENCY,
		true,
		false,
		VERBOSE,
	)
	if err != nil {
		log.Panicln(err)
	}

}

func handleTx(ctx *lib.IndexContext) error {
	ordinals.CalculateOrigins(ctx)
	ordinals.ParseInscriptions(ctx)
	for _, txo := range ctx.Txos {
		if _, ok := txo.Data["bsv20"]; ok || txo.Satoshis != 1 {
			continue
		}
		if txo.Origin == nil {
			log.Panicf("No origin found for %s", txo.Outpoint)
		}
		list := ordlock.ParseScript(txo)
		if list == nil {
			continue
		}
		txo.AddData("list", list)
		originData, err := lib.LoadTxoData(txo.Origin)
		if err != nil {
			log.Panic(err)
		}
		if originData == nil || originData["insc"] == nil {
			rawtx, err := lib.LoadRawtx(hex.EncodeToString(txo.Origin.Txid()))
			if err != nil {
				log.Panic(err)
			}
			ordinals.IndexTxn(rawtx, "", 0, 0)
		}
		txo.Save()
		list.Save(txo)
	}
	return nil
}
