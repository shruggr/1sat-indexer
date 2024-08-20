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
	"github.com/shruggr/1sat-indexer/ingest"
	"github.com/shruggr/1sat-indexer/lib"
)

var THREADS uint64 = 16

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint64
	Transaction []byte
}

func init() {
	godotenv.Load("../../.env")

	var err error
	db, err := pgxpool.New(
		context.Background(),
		os.Getenv("POSTGRES_FULL"),
	)
	if err != nil {
		log.Panic(err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	cache := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISCACHE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb, cache)

	if err != nil {
		log.Panic(err)
	}

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
		if err != nil {
			log.Panic(err)
		}
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
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	ch := sub.Subscribe(context.Background(), "submit", "broadcast").Channel()

	go func() {
		for msg := range ch {
			switch msg.Channel {
			case "submit":
				go func(txid string) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Println("Recovered in submit", r)
						}
					}()
					for i := 0; i < 4; i++ {
						rawtx, err := lib.LoadRawtx(txid)
						if err == nil {
							txCtx, err := ingest.IngestRawtx(rawtx)
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
				}(msg.Payload)
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
					txCtx, err := ingest.IngestRawtx(rawtx)
					log.Printf("[INJEST]: %x %+v\n", txCtx.Txid, err)
				}(rawtx)
			}
		}
	}()

	<-make(chan struct{})
}
