package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GorillaPool/go-junglebus/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

const PAGE_SIZE = uint64(10000)

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
	fromBlock := uint64(1)
	if lastBlocks, err := lib.Rdb.ZPopMax(ctx, "block-sync", 1).Result(); err != nil {
		log.Panic(err)
	} else if len(lastBlocks) == 1 && lastBlocks[0].Score > 6 {
		fromBlock = uint64(lastBlocks[0].Score) - 5
	}

	for {
		blocksSynced, err := lib.Rdb.LLen(ctx, "blocks").Uint64()
		if err != nil {
			log.Panic(err)
		}

		var blocks []*models.BlockHeader
		url := fmt.Sprintf("%s/v1/block_header/list/%d?limit=%d", JUNGLEBUS, fromBlock, PAGE_SIZE)
		log.Printf("Requesting %d blocks from height %d\n", PAGE_SIZE, fromBlock)
		if resp, err := http.Get(url); err != nil || resp.StatusCode != 200 {
			log.Panicln("Failed to get blocks from junglebus", resp.StatusCode, err)
		} else {
			err := json.NewDecoder(resp.Body).Decode(&blocks)
			resp.Body.Close()
			if err != nil {
				log.Panic(err)
			}
		}
		if _, err := lib.Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, block := range blocks {
				if blockJson, err := json.Marshal(block); err != nil {
					log.Panic(err)
				} else {
					if blocksSynced <= uint64(block.Height) {
						pipe.RPush(ctx, "blocks", "-")
						blocksSynced++
					}
					pipe.LSet(ctx, "blocks", int64(block.Height), blockJson)
					pipe.Set(ctx, "chaintip", blockJson, 0)
					pipe.ZAdd(ctx, "block-sync", redis.Z{
						Score:  float64(block.Height),
						Member: block.Hash,
					})
				}
				fromBlock = uint64(block.Height - 5)
			}
			return nil
		}); err != nil {
			log.Panic(err)
		}
		if len(blocks) < int(PAGE_SIZE) {
			time.Sleep(15 * time.Second)
		}
	}
}
