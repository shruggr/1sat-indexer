package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

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

	flag.StringVar(&INDEXER, "id", "", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	var err error
	// log.Println("POSTGRES:", POSTGRES)
	db, err = pgxpool.New(context.Background(), POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

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
	err := indexer.Exec(
		true,
		true,
		handleTx,
		func(height uint32) error {
			return nil
		},
		INDEXER,
		TOPIC,
		FROM_BLOCK,
		CONCURRENCY,
		false,
		false,
		VERBOSE,
	)
	if err != nil {
		log.Panicln(err)
	}

}

type Sale struct {
	Spend    lib.ByteString `json:"spend"`
	Outpoint *lib.Outpoint  `json:"outpoint"`
	Price    uint64         `json:"price,omitempty"`
	Tick     *string        `json:"tick,omitempty"`
	Id       *lib.Outpoint  `json:"id,omitempty"`
	Amt      uint64         `json:"amt,omitempty"`
	Seller   *lib.PKHash    `json:"seller,omitempty"`
	PricePer float64        `json:"pricePer,omitempty"`
}

func handleTx(ctx *lib.IndexContext) error {
	ordinals.IndexInscriptions(ctx)
	ordinals.IndexBsv20(ctx)

	if !bytes.Contains(*ctx.Tx.Inputs[0].UnlockingScript, ordlock.OrdLockSuffix) {
		return nil
	}

	if _, ok := ctx.Txos[0].Data["bsv20"].(*ordinals.Bsv20); ok {
		rows, err := db.Query(context.Background(), `
			UPDATE bsv20_txos
			SET sale=true, spend_height=$2, spend_idx=$3
			WHERE spend=$1
			RETURNING txid, vout, tick, id, price, amt, pkhash, price_per_token`,
			ctx.Txid,
			ctx.Height,
			ctx.Idx,
		)
		if err != nil {
			log.Panicln(err)
		}
		defer rows.Close()
		for rows.Next() {
			var txid []byte
			var vout uint32
			bsv20Sale := &Sale{
				Spend: ctx.Txid,
			}
			err := rows.Scan(&txid, &vout, &bsv20Sale.Tick, &bsv20Sale.Id, &bsv20Sale.Price, &bsv20Sale.Amt, &bsv20Sale.Seller, &bsv20Sale.PricePer)
			if err != nil {
				log.Panicln(err)
			}
			bsv20Sale.Outpoint = lib.NewOutpoint(txid, vout)
			out, err := json.Marshal(bsv20Sale)
			if err != nil {
				log.Panicln(err)
			}
			log.Println("PUBLISHING BSV20 SALE", string(out))
			rdb.Publish(context.Background(), "bsv20sale", out)
		}
	} else {
		var txid []byte
		var vout uint32
		ordSale := &Sale{
			Spend: ctx.Txid,
			Amt:   1,
			// Seller: &lib.PKHash{},
		}
		log.Println("ORDLOCK", hex.EncodeToString(ctx.Txid))
		if rows, err := db.Query(context.Background(), `
			UPDATE listings
			SET sale=true, spend_height=$2, spend_idx=$3
			WHERE spend=$1
			RETURNING txid, vout, price, pkhash`,
			ctx.Txid,
			ctx.Height,
			ctx.Idx,
		); err != nil {
			log.Panicln(err)
		} else if rows.Next() {
			if err := rows.Scan(&txid, &vout, &ordSale.Price, &ordSale.Seller); err != nil {
				log.Panicln(err)
			}
			ordSale.PricePer = float64(ordSale.Price)
			ordSale.Outpoint = lib.NewOutpoint(txid, vout)
			if out, err := json.Marshal(ordSale); err != nil {
				log.Panicln(err)
			} else {
				log.Println("PUBLISHING ORD SALE", string(out))
				rdb.Publish(context.Background(), "ordSale", out)
			}
		}
	}
	return nil
}
