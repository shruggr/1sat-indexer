package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

var dryRun = false

var hexId = "3258136ce310066536524c67bd88d91ed4662e0a1644dd1a17f93c3c3e038541"

func main() {
	godotenv.Load("../.env")
	var err error
	log.Println("POSTGRES_FULL:", os.Getenv("POSTGRES_FULL"))

	db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
	if err != nil {
		log.Panic(err)
	}
	defer db.Close()

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// err = ordinals.Initialize(db, rdb)
	// if err != nil {
	// 	log.Panic(err)
	// }

	// err = ordlock.Initialize(db, rdb)
	// if err != nil {
	// 	log.Panic(err)
	// }

	// rawtx, err := lib.LoadRawtx(hexId)
	// if err != nil {
	// 	panic(err)
	// }
	// ctx, err := lib.ParseTxn(rawtx, "", 0, 0)
	// if err != nil {
	// 	panic(err)
	// }
	// ctx.SaveSpends()
	// ordinals.CalculateOrigins(ctx)
	// ordinals.ParseInscriptions(ctx)
	// lock.ParseLocks(ctx)
	// ordlock.ParseOrdinalLocks(ctx)
	// ctx.Save()

	rows, err := db.Query(context.Background(), `
		SELECT txid, status, op
		FROM bsv20_txos t
		WHERE spend IN (
			SELECT txid
			FROM bsv20_txos t
			JOIN bsv20_subs s ON s.tick=t.tick
			WHERE op='transfer' AND status=-1
		)`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var txid []byte
		var status int
		var op string
		err = rows.Scan(&txid, &status, &op)
		if err != nil {
			panic(err)
		}

		for {
			if op == "mint" {
				if status == -1 {
					// log.Printf("Invalid Mint: %x\n", txid)
				} else if status == 0 {
					log.Printf("Pending Mint: %x\n", txid)
				} else {
					log.Printf("Valid Mint: %x\n", txid)
				}
				break
			}
			row := db.QueryRow(context.Background(), `
				SELECT txid, status, op
				FROM bsv20_txos
				WHERE spend = $1`,
				txid,
			)
			err = row.Scan(&txid, &status, &op)
			if err == pgx.ErrNoRows {
				log.Printf("Investigate: %x\n", txid)
				break
			}
		}
	}

	// token := ordinals.IndexBsv20(ctx)
	// if token == nil {
	// 	for _, txo := range ctx.Txos {
	// 		if list, ok := txo.Data["list"].(*ordlock.Listing); ok {
	// 			list.Save(txo)
	// 		}
	// 	}
	// } else {
	// 	if token.Ticker != "" {
	// 		rdb.Publish(context.Background(), "v1xfer", token.Ticker)
	// 	} else {
	// 		rdb.Publish(context.Background(), "v2xfer", "")
	// 	}
	// }

	// out, err := json.MarshalIndent(txnCtx, "", "  ")

	// if err != nil {
	// 	log.Panic(err)
	// }

	// fmt.Println(string(out))
}
