package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
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

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	var chaintip *models.BlockHeader
	chaintip, err := getChaintip()
	if err != nil {
		log.Panicln(err)
	}
	go func() {
		timer := time.NewTicker(1 * time.Minute)
		for {
			<-timer.C
			chaintip, err = getChaintip()
			if err != nil {
				log.Println("JB Tip", err)
				continue
			}
		}
	}()

	err = indexer.Exec(
		true,
		false,
		func(ctx *lib.IndexContext) error {
			ordinals.ParseInscriptions(ctx)
			ticks := map[string]uint64{}
			for _, txo := range ctx.Txos {
				if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
					ticker := bsv20.Ticker
					if ticker == "" {
						if bsv20.Id == nil {
							continue
						}
						ticker = bsv20.Id.String()
					}
					if txouts, ok := ticks[ticker]; !ok {
						ticks[ticker] = 1
					} else {
						ticks[ticker] = txouts + 1
					}
				}
			}
			for ticker, txouts := range ticks {
				id, err := lib.NewOutpointFromString(ticker)
				if err != nil {
					_, err = db.Exec(context.Background(), `
						INSERT INTO bsv20v1_txns(txid, tick, height, idx, txouts)
						VALUES($1, $2, $3, $4, $5)
						ON CONFLICT(txid, tick) DO NOTHING`,
						ctx.Txid,
						ticker,
						ctx.Height,
						ctx.Idx,
						txouts,
					)
				} else {
					_, err = db.Exec(context.Background(), `
						INSERT INTO bsv20v2_txns(txid, id, height, idx, txouts)
						VALUES($1, $2, $3, $4, $5)
						ON CONFLICT(txid, id) DO NOTHING`,
						ctx.Txid,
						id,
						ctx.Height,
						ctx.Idx,
						txouts,
					)
				}
				if err != nil {
					log.Printf("Err: %s %x %d\n", ticker, ctx.Txid, txouts)
					return err
				}
			}
			return nil
		},
		func(height uint32) error {
			for height > chaintip.Height-6 {
				time.Sleep(5 * time.Second)
			}
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

func getChaintip() (*models.BlockHeader, error) {
	url := fmt.Sprintf("%s/v1/block_header/tip", os.Getenv("JUNGLEBUS"))
	resp, err := http.Get(url)
	if err != nil {
		log.Println("JB Tip Request", err)
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("JB Tip Read", err)
		return nil, err
	}
	chaintip := &models.BlockHeader{}
	err = json.Unmarshal(body, &chaintip)
	if err != nil {
		log.Println("JB Tip Unmarshal", err)
		return nil, err
	}
	fmt.Println("Chaintip", chaintip.Height)
	return chaintip, nil
}
