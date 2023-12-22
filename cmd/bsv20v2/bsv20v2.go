package main

import (
	"context"
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
	// var settled = make(chan uint32, 1000)
	// go func() {
	// 	for height := range settled {

	// 		// var settled uint32
	// 		// if height > 6 {
	// 		// 	settled = height - 6
	// 		// }
	// 		// if tick.Valid && tick.String != "" {
	// 		// 	ordinals.ValidateBsv20DeployTick(settled, tick.String)
	// 		// 	ordinals.ValidateBsv20Mints(settled, tick.String)
	// 		// 	ordinals.ValidateBsv20Transfers(tick.String, height, 8)
	// 		// } else if len(id) > 0 {
	// 		// 	tokenId := lib.Outpoint(id)
	// 		// 	ordinals.ValidateBsv20V2Transfers(&tokenId, height, 8)
	// 		// }
	// 	}
	// }()

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
		true,
		VERBOSE,
	)
	if err != nil {
		panic(err)
	}
}

func handleTx(tx *lib.IndexContext) error {
	ordinals.ParseInscriptions(tx)
	for _, txo := range tx.Txos {
		if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
			if bsv20.Ticker != "" {
				continue
			}
			list := ordlock.ParseScript(txo)

			if list != nil {
				txo.PKHash = list.PKHash
				bsv20.PKHash = list.PKHash
				bsv20.Price = list.Price
				bsv20.PayOut = list.PayOut
				bsv20.Listing = true
				token := ordinals.LoadTokenById(bsv20.Id)
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
}
