package main

import (
	"context"
	"database/sql"
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
	"github.com/shruggr/1sat-indexer/ordlock"
)

// var settled = make(chan uint32, 1000)
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

	// flag.IntVar(&CONCURRENCY, "c", 32, "Concurrency Limit")
	// flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	// flag.Parse()

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
	rows, err := db.Query(context.Background(),
		`SELECT seq, name, tick, id, topic, progress 
		FROM bsv20_subs
		WHERE topic IS NOT NULL`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	var wg sync.WaitGroup
	for rows.Next() {
		var seq uint
		var name string
		var tick sql.NullString
		var id []byte
		var topic string
		var progress uint32

		err := rows.Scan(&seq, &name, &tick, &id, &topic, &progress)
		if err != nil {
			log.Panicln(err)
		}

		wg.Add(1)
		go func(seq uint, name string, tick sql.NullString, id []byte, topic string, progress uint32) {
			var settled = make(chan uint32, 1000)
			go func() {
				for height := range settled {
					var settled uint32
					if height > 6 {
						settled = height - 6
					}
					if tick.Valid && tick.String != "" {
						ordinals.ValidateBsv20DeployTick(settled, tick.String)
						ordinals.ValidateBsv20Mints(settled, tick.String)
						ordinals.ValidateBsv20Transfers(tick.String, height, 8)
						// } else if len(id) > 0 {
						// 	tokenId := lib.Outpoint(id)
						// 	ordinals.ValidateBsv20V2Transfers(&tokenId, height, 8)
					}
				}
			}()

			if progress < lib.TRIGGER {
				progress = lib.TRIGGER
			}
			for {
				err = indexer.Exec(
					true,
					false,
					handleTx,
					func(height uint32) error {
						settled <- height
						_, err := db.Exec(context.Background(), `
							UPDATE bsv20_subs
							SET progress=$2
							WHERE seq=$1 AND progress < $2`,
							seq,
							height,
						)
						if err != nil {
							log.Panicln(err)
						}
						return err
					},
					"",
					topic,
					uint(progress),
					CONCURRENCY,
					true,
					true,
					0,
				)
				if err != nil {
					log.Println("Subscription Error:", topic, err)
				}
			}

		}(seq, name, tick, id, topic, progress)
	}
	rows.Close()
	wg.Wait()

}

func handleTx(tx *lib.IndexContext) error {
	ordinals.ParseInscriptions(tx)
	for _, txo := range tx.Txos {
		if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
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
}

// func handleBlock(height uint32) error {

// 	return nil
// }
