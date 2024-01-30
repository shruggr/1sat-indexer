package main

import (
	"context"
	"encoding/hex"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client

// var TICK string

func init() {
	// wd, _ := os.Getwd()
	// log.Println("CWD:", wd)
	godotenv.Load("../../.env")

	// flag.StringVar(&TICK, "tick", "", "Ticker")
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
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	log.Println("JUNGLEBUS:", os.Getenv("JUNGLEBUS"))

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
	rows, err := db.Query(context.Background(), `
		SELECT txid, vout, height, idx
		FROM implied
		ORDER BY height, idx, vout`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		var txid []byte
		var vout uint32
		var height uint32
		var idx uint64
		err := rows.Scan(&txid, &vout, &height, &idx)
		if err != nil {
			log.Printf("Implied: %x %d\n", txid, vout)
			log.Panicln(err)
		}

		txn, err := lib.JB.GetTransaction(context.Background(), hex.EncodeToString(txid))
		if err != nil {
			log.Printf("Implied: %x %d\n", txid, vout)
			panic(err)
		}

		ctx := ordinals.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
		for _, txo := range ctx.Txos {
			if _, ok := txo.Data["bsv20"]; ok {
				ordinals.IndexBsv20(ctx)
				log.Printf("Not Implied: %x %d\n", txid, vout)
				return
			}
		}

		txo := ctx.Txos[vout]

		txoRow := db.QueryRow(context.Background(), `
			SELECT tick, amt 
			FROM bsv20_txos
			WHERE spend=$1
			ORDER BY vout
			LIMIT 1`,
			txid,
		)

		b := &ordinals.Bsv20{
			Op:      "transfer",
			Implied: true,
		}
		err = txoRow.Scan(&b.Ticker, &b.Amt)
		if err != nil {
			if err != pgx.ErrNoRows {
				log.Printf("Implied: %x %d\n", txid, vout)
				panic(err)
			}
			for _, spend := range ctx.Spends {
				txn, err := lib.JB.GetTransaction(context.Background(), hex.EncodeToString(spend.Outpoint.Txid()))
				if err != nil {
					log.Printf("Implied Parent: %x %d\n", txid, vout)
					panic(err)
				}
				ctx := ordinals.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
				for _, txo := range ctx.Txos {
					if _, ok := txo.Data["bsv20"]; ok {
						ordinals.IndexBsv20(ctx)
						break
					}
				}
			}
			txoRow := db.QueryRow(context.Background(), `
				SELECT tick, amt 
				FROM bsv20_txos
				WHERE spend=$1
				ORDER BY vout
				LIMIT 1`,
				txid,
			)
			err = txoRow.Scan(&b.Ticker, &b.Amt)
			if err != nil {
				log.Printf("Implied Missing Parents: %x %d\n", txid, vout)
				panic(err)
			}
		}
		txo.AddData("bsv20", b)
		ordinals.IndexBsv20(ctx)

		log.Printf("Saving Implied: %s\n", txo.Outpoint.String())
		_, err = db.Exec(context.Background(), `
			UPDATE bsv20_txos
			SET implied=true
			WHERE txid=$1 AND vout=$2`,
			txid,
			vout,
		)

		if err != nil {
			log.Panicf("%x %v", txid, err)
		}
	}
}
