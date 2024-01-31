package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client

var ctx = context.Background()

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

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	prevOrigin, _ := os.ReadFile("./progress.bin")
	// prevOrigin, _ := hex.DecodeString("02062d0fbd6c4eaad76c2b548e5d4fdf6e6081b3b42dea9281364295cdf3e718_36")
	var height sql.NullInt64
	for {
		rows, err := db.Query(ctx, `SELECT origin, outpoint, height
			FROM txos
			WHERE origin IS NOT NULL AND ((origin = $1 AND height >= $2) OR origin > $1) AND spend='\x' AND height IS NOT NULL AND height > 0
			ORDER BY origin, height, idx
			LIMIT 1000`,
			prevOrigin,
			height,
		)
		if err != nil {
			log.Panicln(err)
		}

		limiter := make(chan struct{}, 16)
		var wg sync.WaitGroup
		rowCount := 0
		for rows.Next() {
			rowCount++
			var origin *lib.Outpoint
			var outpoint *lib.Outpoint
			err := rows.Scan(&origin, &outpoint, &height)
			if err != nil {
				log.Panicln(err)
			}
			if !bytes.Equal(*origin, prevOrigin) {
				log.Println("Origin:", origin.String())
				os.WriteFile("./progress.bin", prevOrigin, 0644)
				prevOrigin = *origin
			}

			wg.Add(1)
			limiter <- struct{}{}
			go func(outpoint *lib.Outpoint) {
				defer func() {
					<-limiter
					wg.Done()
				}()

				spend, err := lib.GetSpend(outpoint)
				if err != nil {
					log.Panicln(err)
				}
				if len(spend) == 0 {
					log.Printf("Unpent: %s\n", outpoint.String())
					return
				}
				log.Printf("Spent: %s\n", outpoint.String())
				_, err = db.Exec(ctx, `UPDATE txos
					SET spend=$2
					WHERE outpoint=$1`,
					outpoint,
					spend,
				)
				if err != nil {
					log.Panicln(err)
				}
			}(outpoint)
		}
		rows.Close()
		wg.Wait()

		if rowCount == 0 {
			time.Sleep(1 * time.Minute)
			panic("no rows")
		}
	}
}
