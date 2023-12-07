package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var settled = make(chan uint32, 1000)
var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client

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
	go func() {
		for height := range settled {
			var settled uint32
			if height > 6 {
				settled = height - 6
			}
			var wg sync.WaitGroup
			wg.Add(2)
			log.Printf("[ORD]: Block %d completions\n", height)
			go func(height uint32) {
				err := ordinals.SetInscriptionNum(height)
				if err != nil {
					log.Panicln("Error processing inscription ids:", err)
				}
				wg.Done()
			}(settled)

			go func(height uint32) {
				ordinals.ValidateBsv20(height, indexer.CONCURRENCY)
				wg.Done()
			}(height)

			wg.Wait()
			ordinals.Db.Exec(context.Background(),
				`INSERT INTO progress(indexer, height)
				VALUES ('settled', $1)
				ON CONFLICT (indexer) DO UPDATE SET height=EXCLUDED.height`,
				settled,
			)
		}
	}()
	err := indexer.Exec(
		true,
		false,
		handleTx,
		handleBlock,
	)
	if err != nil {
		panic(err)
	}
}

func handleTx(tx *lib.IndexContext) error {
	ordinals.IndexInscriptions(tx)
	return nil
}

func handleBlock(height uint32) error {
	settled <- height
	return nil
}
