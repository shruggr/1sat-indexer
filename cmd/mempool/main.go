package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/lock"
	"github.com/shruggr/1sat-indexer/ordinals"
	"github.com/shruggr/1sat-indexer/ordlock"
)

var THREADS uint64 = 16

var db *pgxpool.Pool

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint64
	Transaction []byte
}

var rdb *redis.Client

func init() {
	godotenv.Load("../../.env")

	var err error
	db, err = pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES_FULL"),
	)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
		if err != nil {
			log.Panic(err)
		}
	}

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordinals.Initialize(indexer.Db, indexer.Rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordlock.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	// var err error
	// fmt.Println("JUNGLEBUS", os.Getenv("JUNGLEBUS"))
	// junglebusClient, err = junglebus.New(
	// 	junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	// )
	// if err != nil {
	// 	log.Panicln(err.Error())
	// }

	fmt.Println("Starting Mempool")
	// go processQueue()
	sub := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	ch := sub.Subscribe(context.Background(), "submit", "broadcast").Channel()

	go func() {
		for msg := range ch {
			switch msg.Channel {
			case "submit":
				txid := msg.Payload
				if len(txid) != 64 {
					continue
				}
				go func(txid string) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Println("Recovered in submit", r)
						}
					}()
					for i := 0; i < 4; i++ {
						rawtx, err := lib.LoadRawtx(txid)
						if err == nil {
							txCtx, err := processTxn(rawtx)
							log.Printf("[INJEST]: %x %+v\n", txCtx.Txid, err)
							break
						}
						log.Printf("[RETRY] %d: %s\n", i, txid)
						switch i {
						case 0:
							time.Sleep(2 * time.Second)
						case 1:
							time.Sleep(10 * time.Second)
						default:
							time.Sleep(30 * time.Second)
						}
					}
				}(txid)
			case "broadcast":
				rawtx, err := base64.StdEncoding.DecodeString(msg.Payload)
				if err != nil {
					continue
				}
				log.Println("[BROADCAST]")
				go func(rawtx []byte) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Println("Recovered in broadcast")
						}
					}()
					txCtx, err := processTxn(rawtx)
					log.Printf("[INJEST]: %x %+v\n", txCtx.Txid, err)
				}(rawtx)
			}
		}
	}()

	<-make(chan struct{})
}

func processTxn(rawtx []byte) (*lib.IndexContext, error) {
	ctx, err := lib.ParseTxn(rawtx, "", 0, 0)
	if err != nil {
		return nil, err
	}
	ctx.SaveSpends()
	ordinals.CalculateOrigins(ctx)
	ordinals.ParseInscriptions(ctx)
	lock.ParseLocks(ctx)
	ordlock.ParseOrdinalLocks(ctx)
	ctx.Save()

	tokens := map[string]struct{}{}
	for _, txo := range ctx.Txos {
		if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
			list := ordlock.ParseScript(txo)

			if list != nil {
				txo.PKHash = list.PKHash
				bsv20.PKHash = list.PKHash
				bsv20.Price = list.Price
				bsv20.PayOut = list.PayOut
				bsv20.Listing = true

				var decimals uint8
				if bsv20.Ticker != "" {
					if token := ordinals.LoadTicker(bsv20.Ticker); token != nil {
						decimals = token.Decimals
					}
				} else if bsv20.Id != nil {
					if token := ordinals.LoadTokenById(bsv20.Id); token != nil {
						decimals = token.Decimals
					}
				}
				bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt) * float64(10^uint64(decimals))
			}
			if bsv20.Ticker != "" {
				tokens[bsv20.Ticker] = struct{}{}
			} else if bsv20.Id != nil {
				tokens[bsv20.Id.String()] = struct{}{}
			}
			bsv20.Save(txo)
		}
	}
	for tick, _ := range tokens {
		if len(tick) <= 16 {
			rdb.Publish(context.Background(), "v1xfer", fmt.Sprintf("%x:%s", ctx.Txid, tick))
		} else {
			rdb.Publish(context.Background(), "v2xfer", fmt.Sprintf("%x:%s", ctx.Txid, tick))
		}
	}

	return ctx, nil
}
