package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var THREADS uint64 = 32

var db *pgxpool.Pool

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES"),
	)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	result, err := db.Exec(context.Background(), `
		UPDATE txos
		SET spend = '\x'
		WHERE spend IN (
			SELECT txid FROM txns
			WHERE height IS NULL AND created < NOW() - interval '6h'
		)`,
	)
	if err != nil {
		log.Panic(err)
	}
	log.Println("Spends Rolled back:", result.RowsAffected())

	result, err = db.Exec(context.Background(), `
		DELETE FROM txos
		WHERE txid IN (
			SELECT txid FROM txns
			WHERE height IS NULL AND created < NOW() - interval '6h'
		)`,
	)
	if err != nil {
		log.Panic(err)
	}
	log.Println("Txos Rolled back:", result.RowsAffected())

	result, err = db.Exec(context.Background(), `
		DELETE FROM txns
		WHERE height IS NULL AND created < NOW() - interval '6h'`,
	)
	if err != nil {
		log.Panic(err)
	}
	log.Println("Txns Rolled back:", result.RowsAffected())
}
