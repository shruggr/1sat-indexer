package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	// "github.com/GorillaPool/go-junglebus/models"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

const PAGE_SIZE = 10000

var JUNGLEBUS string
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	var err error
	if err = lib.Initialize(); err != nil {
		log.Panic(err)
	}
}
func main() {
	fromBlock := uint64(1)
	if lastBlocks, err := lib.Rdb.ZRevRangeByScoreWithScores(ctx, lib.BlockHeightKey, &redis.ZRangeBy{
		Max:   "+inf",
		Min:   "-inf",
		Count: 1,
	}).Result(); err != nil {
		log.Panic(err)
	} else if len(lastBlocks) == 0 {
		genesis := lib.BlockHeader{
			Hash:       "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f",
			Coin:       1,
			Height:     0,
			Time:       1231006505,
			Nonce:      2083236893,
			Version:    1,
			MerkleRoot: "4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b",
			Bits:       "1d00ffff",
			Synced:     1,
		}
		if _, err := lib.Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			if err := pipe.HSet(ctx, lib.BlockHeadersKey, genesis.Hash, genesis).Err(); err != nil {
				return err
			} else if err := lib.Rdb.ZAdd(ctx, lib.BlockHeightKey, redis.Z{
				Score:  0,
				Member: genesis.Hash,
			}).Err(); err != nil {
				return err
			}
			return nil
		}); err != nil {
			log.Panic(err)
		}

	} else if lastBlocks[0].Score > 6 {
		fromBlock = uint64(lastBlocks[0].Score) - 5
	}

	var err error
	var blocks []*lib.BlockHeader
	for {
		if blocks, err = lib.FetchBlockHeaders(fromBlock, PAGE_SIZE); err != nil {
			log.Panic(err)
		}
		if _, err := lib.Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			var chaintip []byte
			for _, block := range blocks {
				score := float64(block.Height)
				if blockJson, err := json.Marshal(block); err != nil {
					return err
				} else if isNew, err := pipe.HSetNX(ctx, lib.BlockHeadersKey, block.Hash, blockJson).Result(); err != nil {
					return err
				} else if err := pipe.ZRemRangeByScore(ctx, lib.BlockHeightKey, strconv.Itoa(int(block.Height)), "+inf").Err(); err != nil {
					log.Panic(err)
				} else if err := pipe.ZAdd(ctx, lib.BlockHeightKey, redis.Z{
					Score:  score,
					Member: block.Hash,
				}).Err(); err != nil {
					log.Panic(err)
				} else if isNew {
					chaintip = blockJson
				}
				fromBlock = uint64(block.Height) - 5
			}
			if len(chaintip) > 0 {
				log.Println("New Chaintip", string(chaintip))
				if err := lib.Rdb.Set(ctx, lib.ChaintipKey, chaintip, 0).Err(); err != nil {
					return err
				}
				lib.Queue.Publish(ctx, "block", chaintip)
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
