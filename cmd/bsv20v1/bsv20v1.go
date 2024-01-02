package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
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

var tickHashes = map[string][]byte{}
var pkhashTicks = map[string]string{}

func main() {
	sub := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	sub1 := sub.Subscribe(context.Background())
	ch1 := sub1.Channel()
	go func() {
		for msg := range ch1 {
			pkhash, _ := hex.DecodeString(msg.Channel)
			ordinals.UpdateBsv20V1Funding([][]byte{pkhash})
			if tick, ok := pkhashTicks[msg.Channel]; ok {
				rdb.Publish(context.Background(), "v1xfer", tick)
			}
		}
	}()

	trig := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	ch2 := trig.Subscribe(context.Background(), "v1xfer").Channel()
	go func() {
		for {
			<-ch2
			ordinals.ValidatePaidBsv20V1Transfers(CONCURRENCY)
		}
	}()

	rows, err := db.Query(ctx, `
		SELECT b.tick, b.fund_pkhash
		FROM bsv20 b
		JOIN bsv20_subs s ON s.tick=b.tick AND b.status=1`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()

	for rows.Next() {
		var tick string
		var pkhash []byte
		err := rows.Scan(&tick, &pkhash)
		if err != nil {
			log.Panicln(err)
		}
		pkhashHex := hex.EncodeToString(pkhash)
		sub1.Subscribe(context.Background(), pkhashHex)
		pkhashTicks[pkhashHex] = tick
		tickHashes[tick] = pkhash
	}
	rows.Close()

	for _, pkhash := range tickHashes {
		ordinals.UpdateBsv20V1Funding([][]byte{pkhash})
	}
	ordinals.ValidatePaidBsv20V1Transfers(CONCURRENCY)

	rows, err = db.Query(context.Background(), `
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

		wg.Add(1)
		go func(topic string, progress uint32) {
			var settled = make(chan uint32, 1000)
			go func() {
				for height := range settled {
					var settled uint32
					if height > 6 {
						settled = height - 6
					}

					ordinals.ValidateBsv20MintsSubs(settled, topic)
				}
			}()

			if progress < lib.TRIGGER {
				progress = lib.TRIGGER
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
								if len(bsv20.FundPKHash) > 0 {
									pkhash := hex.EncodeToString(bsv20.FundPKHash)
									sub1.Subscribe(context.Background(), pkhash)
									tickHashes[bsv20.Ticker] = bsv20.FundPKHash
									pkhashTicks[pkhash] = bsv20.Ticker
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
								if bsv20.Op == "transfer" {
									rdb.Publish(context.Background(), "v1xfer", bsv20.Ticker)
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

		}(topic, progress)
	}
	rows.Close()
	wg.Wait()

}

// func handleBlock(height uint32) error {

// 	return nil
// }
