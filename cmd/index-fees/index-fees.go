package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2/bscript"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client
var INDEXER string
var TOPIC string
var FROM_BLOCK uint
var VERBOSE int
var CONCURRENCY int = 64

// var ctx = context.Background()

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
}

func main() {
	initializeV1Funding()
	initializeV2Funding()

	err := indexer.Exec(
		true,
		false,
		func(ctx *lib.IndexContext) error {
			return nil
		},
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
		log.Panicln(err)
	}
}

func initializeV1Funding() {
	limiter := make(chan struct{}, CONCURRENCY)
	var wg sync.WaitGroup
	rows, err := db.Query(context.Background(), `
		SELECT DISTINCT fund_pkhash
		FROM bsv20`,
	)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	fmt.Println("Processing v1 funding")
	for rows.Next() {
		var pkhash []byte
		err = rows.Scan(&pkhash)
		if err != nil {
			panic(err)
		}
		limiter <- struct{}{}
		wg.Add(1)
		go func(pkhash []byte) {
			defer func() {
				wg.Done()
				<-limiter
			}()
			add, err := bscript.NewAddressFromPublicKeyHash(pkhash, true)
			if err != nil {
				log.Panicln(err)
			}
			url := fmt.Sprintf("%s/ord/%s", os.Getenv("INDEXER"), add.AddressString)
			log.Println("URL:", url)
			resp, err := http.Get(url)
			if err != nil {
				log.Panicln(err)
			}
			defer resp.Body.Close()
		}(pkhash)
	}
	wg.Wait()
}

func initializeV2Funding() {
	limiter := make(chan struct{}, CONCURRENCY)
	var wg sync.WaitGroup
	rows, err := db.Query(context.Background(), `
		SELECT fund_pkhash
		FROM bsv20_v2`,
	)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	fmt.Println("Processing v2 funding")
	for rows.Next() {
		var pkhash []byte
		err = rows.Scan(&pkhash)
		if err != nil {
			panic(err)
		}
		limiter <- struct{}{}
		wg.Add(1)
		go func(pkhash []byte) {
			defer func() {
				wg.Done()
				<-limiter
			}()
			add, err := bscript.NewAddressFromPublicKeyHash(pkhash, true)
			if err != nil {
				log.Panicln(err)
			}
			url := fmt.Sprintf("%s/ord/%s", os.Getenv("INDEXER"), add.AddressString)
			log.Println("URL:", url)
			resp, err := http.Get(url)
			if err != nil {
				log.Panicln(err)
			}
			defer resp.Body.Close()
		}(pkhash)
	}
	wg.Wait()
}
