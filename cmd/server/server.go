package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GorillaPool/go-junglebus/models"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/cmd/server/sse"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var CONCURRENCY int
var PORT int

var ctx = context.Background()

// var jb *junglebus.Client

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	var err error
	if err = lib.Initialize(); err != nil {
		log.Panic(err)
	}
}

var currentSessions = sse.SessionsLock{
	Topics: make(map[string][]*sse.Session),
}

var tags = []string{
	(&bopen.InscriptionIndexer{}).Tag(),
	(&bopen.MapIndexer{}).Tag(),
	(&bopen.BIndexer{}).Tag(),
	(&bopen.SigmaIndexer{}).Tag(),
	(&bopen.OriginIndexer{}).Tag(),
}

func main() {
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.Parse()

	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(compress.New())

	app.Get("/v1/blocks/tip", func(c *fiber.Ctx) error {
		if tip, err := lib.Cache.Get(ctx, lib.ChaintipKey).Result(); err != nil {
			return err
		} else {
			c.Set("Content-Type", "application/json")
			return c.SendString(tip)
		}
	})

	app.Get("/v1/blocks/:hashOrHeight", func(c *fiber.Ctx) error {
		hashOrHeight := c.Params("hashOrHeight")
		if len(hashOrHeight) <= 8 {
			if blocks, err := lib.Cache.ZRangeByScore(ctx, lib.BlockHeightKey, &redis.ZRangeBy{
				Min:   hashOrHeight,
				Max:   hashOrHeight,
				Count: 1,
			}).Result(); err != nil {
				return err
			} else if len(blocks) == 0 {
				return c.SendStatus(404)
			} else {
				hashOrHeight = blocks[0]
			}
		} else if len(hashOrHeight) != 64 {
			return c.SendStatus(400)
		}
		if block, err := lib.Cache.HGet(ctx, lib.BlockHeadersKey, hashOrHeight).Result(); err != nil {
			return c.SendStatus(404)
		} else {
			c.Set("Content-Type", "application/json")
			return c.SendString(block)
		}
	})

	app.Get("/v1/blocks/list/:from", func(c *fiber.Ctx) error {
		from := c.Params("from")
		blocks := make([]*models.BlockHeader, 0, 10000)
		if hashes, err := lib.Cache.ZRangeByScore(ctx, lib.BlockHeightKey, &redis.ZRangeBy{
			Min:   from,
			Max:   "+inf",
			Count: 10000,
		}).Result(); err != nil {
			return err
		} else if len(hashes) > 0 {
			if blockJsons, err := lib.Cache.HMGet(c.Context(), lib.BlockHeadersKey, hashes...).Result(); err != nil {
				return err
			} else {
				for _, blockJson := range blockJsons {
					var block models.BlockHeader
					if err := json.Unmarshal([]byte(blockJson.(string)), &block); err != nil {
						return err
					}
					blocks = append(blocks, &block)
				}
			}
		}
		return c.JSON(blocks)
	})

	app.Get("/v1/tx/:txid", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if rawtx, err := lib.LoadRawtx(c.Context(), txid); err != nil {
			return err
		} else if len(rawtx) == 0 {
			return c.SendStatus(404)
		} else {
			c.Set("Content-Type", "application/octet-stream")
			buf := bytes.NewBuffer([]byte{})
			buf.Write(transaction.VarInt(uint64(len(rawtx))).Bytes())
			buf.Write(rawtx)
			if proof, err := lib.LoadProof(c.Context(), txid); err != nil {
				return err
			} else if proof == nil {
				buf.Write(transaction.VarInt(0).Bytes())
			} else {
				bin := proof.Bytes()
				buf.Write(transaction.VarInt(uint64(len(bin))).Bytes())
				buf.Write(bin)
			}

			return c.Send(buf.Bytes())
		}
	})

	app.Get("/v1/tx/:txid/raw", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if rawtx, err := lib.LoadRawtx(c.Context(), txid); err != nil {
			return err
		} else if rawtx == nil {
			return c.SendStatus(404)
		} else {
			c.Set("Content-Type", "application/octet-stream")
			return c.Send(rawtx)
		}
	})

	app.Get("/v1/tx/:txid/proof", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if proof, err := lib.LoadProof(c.Context(), txid); err != nil {
			return err
		} else if proof == nil {
			return c.SendStatus(404)
		} else {
			c.Set("Content-Type", "application/octet-stream")
			return c.Send(proof.Bytes())
		}
	})

	app.Get("/v1/txo/:outpoint", func(c *fiber.Ctx) error {
		if txo, err := lib.LoadTxo(c.Context(), c.Params("outpoint"), tags); err != nil {
			return err
		} else if txo == nil {
			return c.SendStatus(404)
		} else {
			return c.JSON(txo)
		}
	})

	// app.Get("/v1/address/:address/:from", func(c *fiber.Ctx) (err error) {
	// 	address := c.Params("address")
	// 	var start float64
	// 	if start, err = strconv.ParseFloat(c.Params("from"), 64); err != nil {
	// 		return err
	// 	}

	// 	scores := make([]float64, 0, 1000)
	// 	txMap := make(map[float64]*lib.TxResult)
	// 	if outpoints, err := lib.Rdb.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
	// 		Key:     lib.OwnerTxosKey(address),
	// 		ByScore: true,
	// 		Start:   start,
	// 		Stop:    "+inf",
	// 		Count:   1000,
	// 	}).Result(); err != nil {
	// 		return err
	// 	} else {
	// 		for _, item := range outpoints {
	// 			var txid string
	// 			var out *uint32
	// 			member := item.Member.(string)
	// 			if len(member) == 64 {
	// 				txid = member
	// 			} else if outpoint, err := lib.NewOutpointFromString(member); err != nil {
	// 				return err
	// 			} else {
	// 				txid = outpoint.TxidHex()
	// 				vout := outpoint.Vout()
	// 				out = &vout
	// 			}
	// 			var result *lib.TxResult
	// 			var ok bool
	// 			if result, ok = txMap[item.Score]; !ok {
	// 				height := uint32(item.Score)
	// 				result = &lib.TxResult{
	// 					Txid:    txid,
	// 					Height:  height,
	// 					Idx:     uint64((item.Score - float64(height)) * 1000000000),
	// 					Outputs: lib.NewOutputMap(),
	// 					Score:   item.Score,
	// 				}

	// 				txMap[item.Score] = result
	// 				scores = append(scores, item.Score)
	// 			}
	// 			if out != nil {
	// 				result.Outputs[*out] = struct{}{}
	// 			}
	// 		}
	// 		slices.Sort(scores)
	// 		results := make([]*lib.TxResult, 0, len(scores))
	// 		for _, score := range scores {
	// 			results = append(results, txMap[score])
	// 		}
	// 		return c.JSON(results)
	// 	}
	// })

	app.Get("/v1/acct/:account/utxos", func(c *fiber.Ctx) (err error) {
		account := c.Params("account")

		if scores, err := lib.Rdb.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
			Key:   lib.AccountTxosKey(account),
			Start: 0,
			Stop:  -1,
		}).Result(); err != nil {
			return err
		} else {
			outpoints := make([]string, 0, len(scores))
			for _, item := range scores {
				member := item.Member.(string)
				if len(member) > 64 {
					outpoints = append(outpoints, member)
				}
			}
			if spends, err := lib.Rdb.HMGet(c.Context(), lib.SpendsKey, outpoints...).Result(); err != nil {
				return err
			} else {
				unspent := make([]string, 0, len(outpoints))
				for i, outpoint := range outpoints {
					if spends[i] == nil {
						unspent = append(unspent, outpoint)
					}
				}
				// TODO: Move this to Txo file
				if msgpacks, err := lib.Rdb.HMGet(c.Context(), lib.TxosKey, unspent...).Result(); err != nil {
					return err
				} else {
					txos := make([]*lib.Txo, 0, len(msgpacks))
					for _, mp := range msgpacks {
						if mp != nil {
							var txo lib.Txo
							if err := json.Unmarshal([]byte(mp.(string)), &txo); err != nil {
								return err
							}
							txos = append(txos, &txo)
						}
					}
					return c.JSON(txos)
				}
			}
		}
	})

	app.Get("/v1/acct/:account/:from", func(c *fiber.Ctx) (err error) {
		account := c.Params("account")
		var start float64
		if start, err = strconv.ParseFloat(c.Params("from"), 64); err != nil {
			return err
		}

		results := make([]*lib.TxResult, 0, 1000)
		txMap := make(map[float64]*lib.TxResult)
		if outpoints, err := lib.Rdb.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
			Key:     lib.AccountTxosKey(account),
			ByScore: true,
			Start:   fmt.Sprintf("(%f", start),
			Stop:    "+inf",
			Count:   1000,
		}).Result(); err != nil {
			return err
		} else {
			for _, item := range outpoints {
				var txid string
				var out *uint32
				member := item.Member.(string)
				if len(member) == 64 {
					txid = member
				} else if outpoint, err := lib.NewOutpointFromString(member); err != nil {
					return err
				} else {
					txid = outpoint.TxidHex()
					vout := outpoint.Vout()
					out = &vout
				}
				var result *lib.TxResult
				var ok bool
				if result, ok = txMap[item.Score]; !ok {
					height := uint32(item.Score / 1000000000)
					result = &lib.TxResult{
						Txid:   txid,
						Height: height,
						// Idx:     uint64((item.Score - float64(height)) * 1000000000),
						Idx:     uint64(item.Score) % 1000000000,
						Outputs: lib.NewOutputMap(),
						Score:   item.Score,
					}

					txMap[item.Score] = result
					results = append(results, result)
				}
				if out != nil {
					result.Outputs[*out] = struct{}{}
				}
			}
		}
		return c.JSON(results)
	})

	app.Put("/v1/acct/:account", func(c *fiber.Ctx) error {
		account := c.Params("account")
		var owners []string
		if err := c.BodyParser(&owners); err != nil {
			return c.SendStatus(400)
		} else if len(owners) == 0 {
			return c.SendStatus(400)
		}
		ownerTxoKeys := make([]string, 0, len(owners))

		resync := false
		accountKey := lib.AccountKey(account)
		for _, owner := range owners {
			if owner == "" {
				return c.SendStatus(400)
			}
			ownerTxoKeys = append(ownerTxoKeys, lib.OwnerTxosKey(owner))
			if err := lib.Rdb.ZAddNX(c.Context(), lib.OwnerSyncKey, redis.Z{
				Score:  0,
				Member: owner,
			}).Err(); err != nil {
				return err
			} else if exists, err := lib.Rdb.SIsMember(c.Context(), accountKey, owner).Result(); err != nil {
				return err
			} else if !exists {
				resync = true
			}
		}
		if resync {
			if _, err := lib.Rdb.Pipelined(c.Context(), func(pipe redis.Pipeliner) error {
				for _, owner := range owners {
					if err := pipe.SAdd(c.Context(), accountKey, owner).Err(); err != nil {
						return err
					} else if err := pipe.HSet(c.Context(), lib.OwnerAccountKey, owner, account).Err(); err != nil {
						return err
					}
				}
				if err := pipe.ZUnionStore(c.Context(), lib.AccountTxosKey(account), &redis.ZStore{
					Keys:      ownerTxoKeys,
					Aggregate: "MIN",
				}).Err(); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return c.SendStatus(204)
	})

	app.Get("/v1/sse", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		topicVal := c.Queries()["topic"]
		topics := strings.Split(topicVal, ",")
		if len(topics) == 0 {
			return c.SendStatus(400)
		}
		log.Println("Subscribing to", topics)

		stateChan := make(chan *redis.Message)

		s := sse.Session{
			Topics:       topics,
			StateChannel: stateChan,
		}

		currentSessions.AddSession(&s)
		keepAliveTickler := time.NewTicker(15 * time.Second)

		defer func() {
			log.Println("Removing Session")
			currentSessions.RemoveSession(&s)
			keepAliveTickler.Stop()
		}()

		// notify := c.Context().Done()

		// c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// keepAliveMsg := ":keepalive\n"

		// 	// listen to signal to close and unregister (doesn't seem to be called)
		// 	go func() {
		// 		<-notify
		// 		log.Printf("Stopped Request\n")
		// 		currentSessions.RemoveSession(&s)
		// 		keepAliveTickler.Stop()
		// 	}()

		for loop := true; loop; {
			select {

			case ev := <-stateChan:
				log.Println("Sending Event", ev.Channel, ev.Payload)
				sseMessage, err := sse.FormatSSEMessage(ev.Channel, ev.Payload)
				if err != nil {
					log.Printf("Error formatting sse message: %v\n", err)
					continue
				}

				// send sse formatted message
				if _, err := c.WriteString(sseMessage); err != nil {
					log.Printf("Error while writing keepalive: %v\n", err)
				}
				// if _, err = fmt.Fprintf(w, sseMessage); err != nil {
				// 	log.Printf("Error while writing Data: %v\n", err)
				// 	continue
				// }

				// err = w.Flush()
				// if err != nil {
				// 	log.Printf("Error while flushing Data: %v\n", err)
				// 	currentSessions.RemoveSession(&s)
				// 	keepAliveTickler.Stop()
				// 	loop = false
				// 	break
				// }
			case <-keepAliveTickler.C:
				if _, err := c.WriteString(":keepalive\n"); err != nil {
					log.Printf("Error while writing keepalive: %v\n", err)
				}
				// fmt.Fprintf(w, keepAliveMsg)
				// err := w.Flush()
				// if err != nil {
				// 	log.Printf("Error while flushing: %v.\n", err)
				// 	currentSessions.RemoveSession(&s)
				// 	keepAliveTickler.Stop()
				// 	loop = false
				// 	break
				// }
			}
		}

		// 	log.Println("Exiting stream")
		// }))

		return nil
	})

	go func() {
		sub := redis.NewClient(&redis.Options{
			Addr:     os.Getenv("REDISQUEUE"),
			Password: "", // no password set
			DB:       0,  // use default DB
		})
		ctx := context.Background()
		pubsub := sub.Subscribe(ctx, "block")
		ch := pubsub.Channel()

		for {
			select {
			case addSubs := <-currentSessions.AddSubs:
				log.Println("Subscribing to", addSubs)
				pubsub.Subscribe(ctx, addSubs...)
			case removedSubs := <-currentSessions.RemoveSubs:
				log.Println("Unsubscribing to", removedSubs)
				pubsub.Unsubscribe(ctx, removedSubs...)
			case msg := <-ch:
				log.Println("Received Message", msg.Channel, msg.Payload)
				for _, session := range currentSessions.Topics[msg.Channel] {
					session.StateChannel <- msg
				}
			}
		}
	}()

	// Temporary fix for unfound memory leak
	// go func() {
	// 	log.Println("Registering Reset Timer")
	// 	resetTimer := time.NewTimer(30 * time.Minute)
	// 	<-resetTimer.C
	// 	log.Println("Resetting")
	// 	os.Exit(0)
	// }()
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}
