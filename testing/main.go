package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var rdb *redis.Client

var dryRun = false

var hexId = "33f71e34bc3998561afe2780b56f680783964d3f4e52857ac3dd00c8185f874e"

func main() {
	// var err error
	err := godotenv.Load("../.env")
	if err != nil {
		log.Panic(err)
	}
	log.Println("POSTGRES_FULL:", os.Getenv("POSTGRES_FULL"))

	db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
	if err != nil {
		log.Panic(err)
	}
	defer db.Close()

	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordinals.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	tx, err := lib.JB.GetTransaction(context.Background(), hexId)
	if err != nil {
		log.Panic(err)
	}

	txnCtx, err := lib.ParseTxn(tx.Transaction, tx.BlockHash, tx.BlockHeight, tx.BlockIndex)
	if err != nil {
		log.Panic(err)
	}
	ordinals.ParseInscriptions(txnCtx)

	ordinals.ParseBsv20(txnCtx)
	// sigil.ParseSigil(txnCtx)

	out, err := json.MarshalIndent(txnCtx, "", "  ")
	if err != nil {
		log.Panic(err)
	}

	// op, err := lib.NewOutpointFromString("2ec2781d815226e925747246b4c10730269da0e431f9edafcd6c12d8726434c6_0")
	// if err != nil {
	// 	log.Panic(err)
	// }
	// current, err := ordinals.GetLatestOutpoint(context.Background(), op)
	// if err != nil {
	// 	log.Panic(err)
	// }

	fmt.Println(string(out))
}
