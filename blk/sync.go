package blk

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/bitcoin-sv/go-sdk/chainhash"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/evt"
)

const PAGE_SIZE = 10000

func Sync(ctx context.Context) {
	fromBlock := uint32(1)
	var lastHash *chainhash.Hash
	if lastBlocks, err := DB.ZRevRangeByScoreWithScores(ctx, BlockHeightKey, &redis.ZRangeBy{
		Max:   "+inf",
		Min:   "-inf",
		Count: 1,
	}).Result(); err != nil {
		log.Panic(err)
	} else if len(lastBlocks) == 0 {
		genesis := BlockHeader{
			Coin:    1,
			Height:  0,
			Time:    1231006505,
			Nonce:   2083236893,
			Version: 1,
			Bits:    "1d00ffff",
			Synced:  1,
		}
		genesis.Hash, _ = chainhash.NewHashFromHex("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		genesis.MerkleRoot, _ = chainhash.NewHashFromHex("4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b")

		hash := genesis.Hash.String()
		if _, err := DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			if err := pipe.HSet(ctx, BlockHeadersKey, hash, genesis).Err(); err != nil {
				return err
			} else if err := DB.ZAdd(ctx, BlockHeightKey, redis.Z{
				Score:  0,
				Member: hash,
			}).Err(); err != nil {
				return err
			}
			return nil
		}); err != nil {
			log.Panic(err)
		}
		lastHash = genesis.Hash
	} else {
		fromBlock = uint32(lastBlocks[0].Score)
		if lastHash, err = chainhash.NewHashFromHex(lastBlocks[0].Member.(string)); err != nil {
			log.Panic(err)
		}
	}

	var err error
	var blocks []*BlockHeader
	for {
		start := uint64(1)
		if fromBlock > 6 {
			start = uint64(fromBlock - 6)
		}
		if blocks, err = FetchBlockHeaders(start, PAGE_SIZE); err != nil {
			log.Panic(err)
		}
		var blockJson []byte
		if _, err := DB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			for _, block := range blocks {
				score := float64(block.Height)
				if blockJson, err = json.Marshal(block); err != nil {
					return err
				} else if err := pipe.HSetNX(ctx, BlockHeadersKey, block.Hash.String(), blockJson).Err(); err != nil {
					return err
				} else if err := pipe.ZRemRangeByScore(ctx, BlockHeightKey, strconv.Itoa(int(block.Height)), "+inf").Err(); err != nil {
					log.Panic(err)
				} else if err := pipe.ZAdd(ctx, BlockHeightKey, redis.Z{
					Score:  score,
					Member: block.Hash.String(),
				}).Err(); err != nil {
					log.Panic(err)
				}
			}
			return nil
		}); err != nil {
			log.Panic(err)
		}
		lastBlock := blocks[len(blocks)-1]

		if !lastBlock.Hash.Equal(*lastHash) {
			if err := DB.Set(ctx, ChaintipKey, string(blockJson), 0).Err(); err != nil {
				log.Panic(err)
			}
			evt.Publish(ctx, ChaintipKey, string(blockJson))
		}
		lastHash = lastBlock.Hash
		fromBlock = lastBlock.Height

		if len(blocks) < int(PAGE_SIZE) {
			time.Sleep(15 * time.Second)
		}
	}
}
