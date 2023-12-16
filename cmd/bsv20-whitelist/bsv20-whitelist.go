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
		`SELECT seq FROM bsv20_subs`,
	)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var wg sync.WaitGroup
	for rows.Next() {
		var seq uint
		err := rows.Scan(&seq)
		if err != nil {
			panic(err)
		}
		wg.Add(1)
		go func(seq uint) {
			row := db.QueryRow(context.Background(), `
				SELECT seq, name, tick, topic, progress 
				FROM bsv20_subs
				WHERE seq=$1`,
				seq,
			)
			if err != nil {
				panic(err)
			}
			var name string
			var tick sql.NullString
			var topic string
			var progress uint32
			err := row.Scan(&seq, &name, &tick, &topic, &progress)
			if err != nil {
				panic(err)
			}

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
					}
				}
			}()

			if progress < lib.TRIGGER {
				progress = lib.TRIGGER
			}
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
						panic(err)
					}
					return err
				},
				"",
				topic,
				uint(progress),
				CONCURRENCY,
				VERBOSE,
			)
			if err != nil {
				panic(err)
			}
		}(seq)
	}
	rows.Close()
	wg.Wait()

}

func handleTx(tx *lib.IndexContext) error {
	ordinals.IndexInscriptions(tx, false)

	for _, txo := range tx.Txos {
		if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
			bsv20.Save(txo)
		}
	}
	return nil
}

// func handleBlock(height uint32) error {

// 	return nil
// }
