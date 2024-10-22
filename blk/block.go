package blk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/GorillaPool/go-junglebus"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var JUNGLEBUS string
var JB *junglebus.Client
var DB *redis.Client

const BlockHeightKey = "blk:height"
const BlockHeadersKey = "blk:headers"
const ChaintipKey = "blk:tip"

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))
	var err error
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
	if JUNGLEBUS != "" {
		log.Println("JUNGLEBUS", JUNGLEBUS)
		JB, err = junglebus.New(
			junglebus.WithHTTP(JUNGLEBUS),
		)
		if err != nil {
			log.Panic(err)
		}
	}

	log.Println("REDISBLK", os.Getenv("REDISBLK"))
	DB = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISBLK"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
}

var sub *redis.Client

func StartChaintipSub(ctx context.Context) (chaintip *BlockHeader, c chan *BlockHeader, err error) {
	c = make(chan *BlockHeader)
	go func(c chan *BlockHeader) {
		log.Println("REDISEVT", os.Getenv("REDISEVT"))
		sub = redis.NewClient(&redis.Options{
			Addr:     os.Getenv("REDISEVT"),
			Password: "", // no password set
			DB:       0,  // use default DB
		})
		pubSub := sub.Subscribe(context.Background(), ChaintipKey)
		ch := pubSub.Channel()
		for {
			select {
			case <-ctx.Done():
				sub.Close()
				return
			case msg := <-ch:
				chaintip := &BlockHeader{}
				if err := json.Unmarshal([]byte(msg.Payload), chaintip); err != nil {
					log.Println("Failed to unmarshal chaintip", err)
				} else {
					c <- chaintip
				}
			}

		}
	}(c)

	chaintip, err = Chaintip(ctx)
	return chaintip, c, err
}

func Chaintip(ctx context.Context) (*BlockHeader, error) {
	header := &BlockHeader{}
	if data, err := DB.Get(ctx, ChaintipKey).Bytes(); err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else if err = json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	return header, nil
}

func BlockByHeight(ctx context.Context, height uint32) (*BlockHeader, error) {
	score := strconv.FormatUint(uint64(height), 10)
	if blocks, err := DB.ZRangeByScore(ctx, BlockHeightKey, &redis.ZRangeBy{
		Min:   score,
		Max:   score,
		Count: 1,
	}).Result(); err != nil {
		return nil, err
	} else if len(blocks) == 0 {
		return nil, nil
	} else {
		return BlockByHash(ctx, blocks[0])
	}
}

func BlockByHash(ctx context.Context, hash string) (*BlockHeader, error) {
	block := &BlockHeader{}
	if data, err := DB.HGet(ctx, BlockHeadersKey, hash).Bytes(); err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else if err = json.Unmarshal(data, &block); err != nil {
		return nil, err
	}
	return block, nil
}

func Blocks(ctx context.Context, fromBlock uint64, count uint) ([]*BlockHeader, error) {
	from := strconv.FormatUint(fromBlock, 10)
	blocks := make([]*BlockHeader, 0, count)
	if hashes, err := DB.ZRangeByScore(ctx, BlockHeightKey, &redis.ZRangeBy{
		Min:   from,
		Max:   "+inf",
		Count: int64(count),
	}).Result(); err != nil {
		return nil, err
	} else if len(hashes) > 0 {
		if blockJsons, err := DB.HMGet(ctx, BlockHeadersKey, hashes...).Result(); err != nil {
			return nil, err
		} else {
			for _, blockJson := range blockJsons {
				var block BlockHeader
				if err := json.Unmarshal([]byte(blockJson.(string)), &block); err != nil {
					return nil, err
				}
				blocks = append(blocks, &block)
			}
		}
	}
	return blocks, nil
}
