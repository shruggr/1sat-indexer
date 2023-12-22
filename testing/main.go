package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bk/bip32"
	"github.com/libsv/go-bk/chaincfg"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var rdb *redis.Client

var dryRun = false

var hexId = "75ea3982da6b23e08167d0b6e70e5b4b5bebf081299b9f54e011ca1c0f60181f"

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

	err = ordinals.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	ek, err := bip32.NewKeyFromString("xpub661MyMwAqRbcF221R74MPqdipLsgUevAAX4hZP2rywyEeShpbe3v2r9ciAvSGT6FB22TEmFLdUyeEDJL4ekG8s9H5WXbzDQPr6eW1zEYYy9")
	if err != nil {
		log.Panic(err)
	}

	fmt.Println(ek.Address(&chaincfg.MainNet))
	ek, err = ek.DeriveChildFromPath("123/123")
	if err != nil {
		log.Panic(err)
	}

	fmt.Println(ek.Address(&chaincfg.MainNet))
	// rawtx, err := lib.LoadRawtx("75ea3982da6b23e08167d0b6e70e5b4b5bebf081299b9f54e011ca1c0f60181f")

	// txnCtx, err := lib.ParseTxn(rawtx, "", 0, 0)

	// ordinals.ParseInscriptions(txnCtx)

	// for _, txo := range txnCtx.Txos {
	// 	if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
	// 		list := ordlock.ParseScript(txo)

	// 		if list != nil {
	// 			txo.PKHash = list.PKHash
	// 			bsv20.PKHash = list.PKHash
	// 			bsv20.Price = list.Price
	// 			bsv20.PayOut = list.PayOut
	// 			bsv20.Listing = true
	// 			var token *ordinals.Bsv20
	// 			if bsv20.Id != nil {
	// 				token = ordinals.LoadTokenById(bsv20.Id)
	// 			} else if bsv20.Ticker != "" {
	// 				token = ordinals.LoadTicker(bsv20.Ticker)
	// 			}
	// 			var decimals uint8
	// 			if token != nil {
	// 				decimals = token.Decimals
	// 			}
	// 			bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt*(10^uint64(decimals)))
	// 		}
	// 		bsv20.Save(txo)
	// 	}
	// }

	// out, err := json.MarshalIndent(txnCtx, "", "  ")

	// if err != nil {
	// 	log.Panic(err)
	// }

	// fmt.Println(string(out))
}
