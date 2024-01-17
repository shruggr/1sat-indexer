package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

// var settled = make(chan uint32, 1000)
var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client
var INDEXER string
var TOPIC string
var FROM_BLOCK uint
var VERBOSE int
var CONCURRENCY int = 64
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	// flag.IntVar(&CONCURRENCY, "c", 32, "Concurrency Limit")
	// flag.IntVar(&VERBOSE, "v", 0, "Verbose")
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

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordinals.Initialize(indexer.Db, indexer.Rdb)
	if err != nil {
		log.Panic(err)
	}
}

var pkhashFunds = map[string]*ordinals.V1TokenFunds{}
var tickFunds map[string]*ordinals.V1TokenFunds

func main() {
	tickFunds = ordinals.UpdateBsv20V1Funding()
	sub := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	sub1 := sub.Subscribe(context.Background(), "v1xfer")
	ch1 := sub1.Channel()
	go func() {
		for msg := range ch1 {
			if msg.Channel == "v1xfer" {
				parts := strings.Split(msg.Payload, ":")
				txid, err := hex.DecodeString(parts[0])
				if err != nil {
					continue
				}
				ordinals.ValidateV1Transfer(txid, parts[1], false)
				continue
			}
			tickFunds = ordinals.UpdateBsv20V1Funding()
		}
	}()

	for _, funds := range tickFunds {
		pkhashHex := hex.EncodeToString(funds.PKHash)
		pkhashFunds[pkhashHex] = funds
		sub1.Subscribe(context.Background(), pkhashHex)
	}

	rows, err := db.Query(context.Background(), `
		SELECT topic, MIN(progress) as progress
		FROM bsv20_subs
		WHERE topic IS NOT NULL
		GROUP BY topic`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	var wg sync.WaitGroup
	for rows.Next() {
		var topic string
		var progress uint32

		err := rows.Scan(&topic, &progress)
		if err != nil {
			log.Panicln(err)
		}

		ordinals.ValidateBsv20MintsSubs(progress-6, topic)
		wg.Add(1)
		go func(topic string, progress uint32) {
			var settled = make(chan uint32, 1000)
			go func() {
				for height := range settled {
					tickFunds = ordinals.UpdateBsv20V1Funding()
					ordinals.ValidateBsv20MintsSubs(height-6, topic)
					ordinals.ValidatePaidBsv20V1Transfers(CONCURRENCY, height)
				}
			}()

			if progress < 807000 {
				progress = 807000
			}
			for {
				err = indexer.Exec(
					true,
					false,
					func(tx *lib.IndexContext) error {
						ordinals.ParseInscriptions(tx)
						for _, txo := range tx.Txos {
							if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
								if bsv20.Ticker == "" {
									continue
								}

								list := ordlock.ParseScript(txo)

								if list != nil {
									txo.PKHash = list.PKHash
									bsv20.PKHash = list.PKHash
									bsv20.Price = list.Price
									bsv20.PayOut = list.PayOut
									bsv20.Listing = true
									var token *ordinals.Bsv20
									if bsv20.Id != nil {
										token = ordinals.LoadTokenById(bsv20.Id)
									} else if bsv20.Ticker != "" {
										token = ordinals.LoadTicker(bsv20.Ticker)
									}
									var decimals uint8
									if token != nil {
										decimals = token.Decimals
									}
									bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt*(10^uint64(decimals)))
								}
								bsv20.Save(txo)
								if _, ok := tickFunds[bsv20.Ticker]; ok && bsv20.Op == "transfer" {
									rdb.Publish(context.Background(), "v1xfer", fmt.Sprintf("%x:%s", tx.Txid, bsv20.Ticker))
								}
							}
						}
						return nil
					},
					func(height uint32) error {
						settled <- height
						_, err := db.Exec(context.Background(), `
							UPDATE bsv20_subs
							SET progress=$2
							WHERE topic=$1 AND progress < $2`,
							topic,
							height,
						)
						if err != nil {
							log.Panicln(err)
						}
						return err
					},
					"",
					topic,
					uint(progress),
					CONCURRENCY,
					true,
					true,
					1,
				)
				if err != nil {
					log.Println("Subscription Error:", topic, err)
				}
			}
		}(topic, progress-6)
	}
	// rdb.Publish(context.Background(), "v1xfer", "")
	rows.Close()
	wg.Wait()

}
