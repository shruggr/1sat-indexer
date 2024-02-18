package main

import (
	"context"
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

var hexId = "ae10ddcb8d976be53b9107701d90b67a189ff6bd8bea8b9e4ed34c5d4ee258f3"

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

	// tx, err := lib.JB.GetTransaction(context.Background(), hexId)
	// if err != nil {
	// 	log.Panic(err)
	// }

	// txnCtx, err := lib.ParseTxn(tx.Transaction, tx.BlockHash, tx.BlockHeight, tx.BlockIndex)
	// if err != nil {
	// 	log.Panic(err)
	// }
	// ordinals.ParseInscriptions(txnCtx)

	// sigil.ParseSigil(txnCtx)

	// out, err := json.MarshalIndent(txnCtx, "", "  ")

	op, err := lib.NewOutpointFromString("2ec2781d815226e925747246b4c10730269da0e431f9edafcd6c12d8726434c6_0")
	if err != nil {
		log.Panic(err)
	}
	current, err := ordinals.GetLatestOutpoint(context.Background(), op)
	if err != nil {
		log.Panic(err)
	}

	fmt.Println(string(current.String()))
}
