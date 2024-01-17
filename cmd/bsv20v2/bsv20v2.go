package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

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
var ctx = context.Background()

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
		Addr:     "localhost:6379",
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
}

var pkhashFunds = map[string]*ordinals.V2TokenFunds{}
var tickFunds map[string]*ordinals.V2TokenFunds

func main() {
	tickFunds = ordinals.UpdateBsv20V2Funding()
	sub := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	sub1 := sub.Subscribe(context.Background())
	ch1 := sub1.Channel()
	go func() {
		for msg := range ch1 {
			row := db.QueryRow(context.Background(), `
				SELECT SUM(satoshis) as total 
				FROM txos
				WHERE pkhash = decode($1, 'hex')`,
				msg.Channel,
			)
			funds := tickFunds[msg.Channel]
			err := row.Scan(&funds.Total)
			if err != nil {
				log.Println(err)
			}
		}
	}()

	for _, funds := range tickFunds {
		pkhashHex := hex.EncodeToString(funds.PKHash)
		pkhashFunds[pkhashHex] = funds
		sub1.Subscribe(context.Background(), pkhashHex)
	}

	trig := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	ch2 := trig.Subscribe(context.Background(), "v2xfer").Channel()
	go func() {
		for msg := range ch2 {
			parts := strings.Split(msg.Payload, ":")
			txid, err := hex.DecodeString(parts[0])
			if err != nil {
				continue
			}
			id, err := lib.NewOutpointFromString(parts[1])
			if err != nil {
				log.Println("Invalid Outpoint:", parts[1])
				continue
			}
			ordinals.ValidateV2Transfer(txid, id, false)
		}
	}()

	// rows, err := db.Query(ctx,
	// 	`SELECT fund_pkhash FROM bsv20_v2`,
	// )
	// if err != nil {
	// 	log.Panicln(err)
	// }
	// defer rows.Close()
	// pkhashes := [][]byte{}
	// for rows.Next() {
	// 	var pkhash []byte
	// 	err := rows.Scan(&pkhash)
	// 	if err != nil {
	// 		log.Panicln(err)
	// 	}
	// 	sub1.Subscribe(context.Background(), hex.EncodeToString(pkhash))
	// 	pkhashes = append(pkhashes, pkhash)
	// }
	// rows.Close()

	// for _, pkhash := range pkhashes {
	// 	ordinals.UpdateBsv20V2Funding([][]byte{pkhash})
	// }

	// var settled = make(chan uint32, 1000)
	// go func() {
	// 	for {
	// 		<-settled
	// 		ordinals.ValidateBsvPaid20V2Transfers(CONCURRENCY)
	// 	}
	// }()
	var settled = make(chan uint32, 1000)
	go func() {
		for height := range settled {
			tickFunds = ordinals.UpdateBsv20V2Funding()
			// ordinals.ValidateBsv20MintsSubs(height-6, topic)
			ordinals.ValidatePaidBsv20V2Transfers(CONCURRENCY, height)
		}
	}()

	err := indexer.Exec(
		true,
		false,
		func(tx *lib.IndexContext) error {
			ordinals.ParseInscriptions(tx)
			for _, txo := range tx.Txos {
				// pkhash := hex.EncodeToString(txo.PKHash)
				if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
					if bsv20.Ticker != "" {
						continue
					}
					if len(bsv20.FundPKHash) > 0 {
						pkhash := hex.EncodeToString(bsv20.FundPKHash)
						sub1.Subscribe(context.Background(), pkhash)
					}
					list := ordlock.ParseScript(txo)

					if list != nil {
						txo.PKHash = list.PKHash
						bsv20.PKHash = list.PKHash
						bsv20.Price = list.Price
						bsv20.PayOut = list.PayOut
						bsv20.Listing = true
						token := ordinals.LoadTokenById(bsv20.Id)
						var decimals uint8
						if token != nil {
							decimals = token.Decimals
						}
						bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt*(10^uint64(decimals)))
					}
					bsv20.Save(txo)
					if bsv20.Op == "transfer" {
						rdb.Publish(context.Background(), "v2xfer", fmt.Sprintf("%x:%s", tx.Txid, bsv20.Id.String()))
					}
				}
			}
			return nil
		},
		func(height uint32) error {
			return nil
		},
		INDEXER,
		TOPIC,
		FROM_BLOCK,
		CONCURRENCY,
		true,
		true,
		VERBOSE,
	)
	if err != nil {
		log.Panicln(err)
	}
}
