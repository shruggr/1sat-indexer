package main

import (
	"context"
	"encoding/hex"
	"log"
	"os"

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
		Addr:     "localhost:6379",
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
	// token := ordinals.LoadTicker(TICK)
	rows, err := db.Query(context.Background(), `
		SELECT txid, vout, height, idx
		FROM implied
		ORDER BY height, idx, vout`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	// limiter := make(chan struct{}, 64)
	// var wg sync.WaitGroup
	for rows.Next() {
		var txid []byte
		var vout uint32
		var height uint32
		var idx uint64
		err := rows.Scan(&txid, &vout, &height, &idx)
		if err != nil {
			log.Panicln(err)
		}

		txn, err := lib.JB.GetTransaction(context.Background(), hex.EncodeToString(txid))
		if err != nil {
			panic(err)
		}
		ctx, err := lib.ParseTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
		if err != nil {
			panic(err)
		}
		ordinals.IndexInscriptions(ctx)
		t := ordinals.IndexBsv20(ctx)
		if t != nil {
			return
		}
		txo := ctx.Txos[vout]

		txoRow := db.QueryRow(context.Background(), `
			SELECT tick, amt 
			FROM bsv20_txos
			WHERE spend=$1 and vout=0`,
			txid,
		)

		// var amt uint64
		bsv20 := &ordinals.Bsv20{
			Op:      "transfer",
			Implied: true,
		}
		err = txoRow.Scan(&bsv20.Ticker, &bsv20.Amt)
		if err != nil {
			panic(err)
		}
		// bsv20.Amt = &amt

		bsv20.Save(txo)
		// // onesats := make([]uint32, 0, len(ctx.Txos))
		// // for vout, txo := range ctx.Txos {
		// // 	if _, ok := txo.Data["bsv20"]; ok {
		// // 		log.Printf("Not Implied %x %d\n", txid, vout)
		// // 		return
		// // 	}
		// // 	if txo.Satoshis == 1 {
		// // 		// satsIn := uint64(0)
		// // 		// for vin, txin := range ctx.Tx.Inputs {
		// // 		// 	inTxn, err := lib.JB.GetTransaction(context.Background(), txin.PreviousTxIDStr())
		// // 		// 	if err != nil {
		// // 		// 		panic(err)
		// // 		// 	}
		// // 		// 	ctx, err := lib.ParseTxn(inTxn.Transaction, inTxn.BlockHash, inTxn.BlockHeight, inTxn.BlockIndex)
		// // 		// 	if err != nil {
		// // 		// 		panic(err)
		// // 		// 	}
		// // 		// 	ordinals.IndexInscriptions(ctx)
		// // 		// 	ordinals.IndexBsv20(ctx)
		// // 		// 	if satsIn == txo.OutAcc {
		// // 		// 	}
		// // 		// 	satsIn += inTx.Outputs[txin.PreviousTxOutIndex].Satoshis
		// // 		// }
		// // 	}
		// // }

		// url := fmt.Sprintf("%s/v1/txo/spend/%x_%d", os.Getenv("JUNGLEBUS"), txid, vout)
		// // log.Println("URL:", url)
		// resp, err := http.Get(url)
		// if err != nil {
		// 	panic(err)
		// }
		// if resp.StatusCode == 404 {
		// 	log.Panicf("Not Found: %s\n", url)
		// }
		// spend, err := io.ReadAll(resp.Body)
		// if err != nil {
		// 	panic(err)
		// }
		// if len(spend) == 0 {
		// 	return
		// }

		// // token := ordinals.IndexBsv20(ctx)
		// // if token != nil {
		// // 	return
		// // }
		// outSats := uint64(0)
		// for _, txo := range ctx.Txos {
		// 	if outSats == outacc && txo.Satoshis == 1 {
		// 		bsv20 := &ordinals.Bsv20{
		// 			Ticker:  TICK,
		// 			Op:      "transfer",
		// 			Amt:     &amt,
		// 			Implied: true,
		// 		}
		// 		list := ordlock.ParseScript(txo)

		// 		if list != nil {
		// 			txo.PKHash = list.PKHash
		// 			bsv20.PKHash = list.PKHash
		// 			bsv20.Price = list.Price
		// 			bsv20.PayOut = list.PayOut
		// 			bsv20.Listing = true

		// 			var decimals uint8
		// 			if token != nil {
		// 				decimals = token.Decimals
		// 			}
		// 			bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt*(10^uint64(decimals)))
		// 		}
		// 		// bsv20.Save(txo)
		// 		log.Printf("Saving Implied: %s\n", txo.Outpoint)
		// 		return
		// 	}
	}
}
