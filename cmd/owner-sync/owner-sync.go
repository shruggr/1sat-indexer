package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var JUNGLEBUS string
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	var err error
	db, err := pgxpool.New(ctx, os.Getenv("POSTGRES_FULL"))
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

	if err = lib.Initialize(db, rdb, cache); err != nil {
		log.Panic(err)
	}
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
}

func main() {
	// indexers := make([]lib.Indexer, 0)
	for {
		iter := lib.Rdb.ZScan(ctx, lib.OwnerSyncKey, 0, "", 100).Iterator()
		for iter.Next(ctx) {
			add := iter.Val()
			iter.Next(ctx)
			var addTxns []*lib.AddressTxn
			log.Println("Address:", add)
			if lastHeight, err := strconv.Atoi(iter.Val()); err != nil {
				log.Panic(err)
			} else if resp, err := http.Get(fmt.Sprintf("%s/v1/address/get/%s/%d", JUNGLEBUS, add, lastHeight)); err != nil {
				log.Panic(err)
			} else if resp.StatusCode != 200 {
				log.Panic("Bad status code", resp.StatusCode)
			} else {
				log.Println("URL:", resp.Request.URL.RequestURI())
				decoder := json.NewDecoder(resp.Body)
				if err := decoder.Decode(&addTxns); err != nil {
					log.Panic(err)
				}
				for _, addTxn := range addTxns {
					score, _ := strconv.ParseFloat(fmt.Sprintf("%07d.%09d", addTxn.Height, addTxn.Idx), 64)
					if err := lib.Rdb.ZAdd(ctx, lib.IngestQueueKey(""), redis.Z{
						Score:  score,
						Member: addTxn.Txid,
					}).Err(); err != nil {
						log.Panic(err)
					}
					log.Println("Queuing", addTxn.Txid, score)
					if addTxn.Height > uint32(lastHeight) {
						lastHeight = int(addTxn.Height)
					}
				}
				if err := lib.Rdb.ZAdd(ctx, lib.OwnerSyncKey, redis.Z{
					Score:  float64(lastHeight),
					Member: add,
				}).Err(); err != nil {
					log.Panic(err)
				}
				log.Println("Queued", add, lastHeight)
			}
		}
		time.Sleep(time.Minute)
	}
}
