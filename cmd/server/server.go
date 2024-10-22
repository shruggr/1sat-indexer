package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/blk"
	"github.com/shruggr/1sat-indexer/broadcast"
	"github.com/shruggr/1sat-indexer/cmd/server/sse"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/jb"
	"github.com/shruggr/1sat-indexer/lib"
)

var POSTGRES string
var CONCURRENCY uint
var VERBOSE int
var TAG string
var PORT int

var currentSessions = sse.SessionsLock{
	Topics: make(map[string][]*sse.Session),
}

var indexedTags = make([]string, 0, len(config.Indexers))

var ingest = &idx.IngestCtx{
	Indexers:    config.Indexers,
	Concurrency: CONCURRENCY,
}

var chaintip *blk.BlockHeader

func init() {
	flag.StringVar(&TAG, "tag", "ingest", "Ingest tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	for _, indexer := range config.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
	go func() {
		if header, c, err := blk.StartChaintipSub(context.Background()); err != nil {
			log.Panic(err)
		} else if header == nil {
			log.Panic("No Chaintip")
		} else {
			log.Println("Chaintip", header.Height, header.Hash)
			chaintip = header
			for header := range c {
				log.Println("Chaintip", header.Height, header.Hash)
				chaintip = header
			}
		}
	}()
}

func main() {
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.Parse()

	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(compress.New())

	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("yo")
	})

	app.Get("/v5/blocks/tip", func(c *fiber.Ctx) error {
		if tip, err := blk.Chaintip(c.Context()); err != nil {
			return err
		} else {
			c.Set("Cache-Control", "no-cache,no-store")
			return c.JSON(tip)
		}
	})

	app.Get("/v5/blocks/height/:height", func(c *fiber.Ctx) error {
		if height, err := strconv.ParseUint(c.Params("height"), 10, 32); err != nil {
			return c.SendStatus(400)
		} else if block, err := blk.BlockByHeight(c.Context(), uint32(height)); err != nil {
			return err
		} else if block == nil {
			return c.SendStatus(404)
		} else {
			if block.Height < chaintip.Height-5 {
				c.Set("Cache-Control", "public,max-age=31536000,immutable")
			} else {
				c.Set("Cache-Control", "public,max-age=60")
			}
			return c.JSON(block)
		}
	})

	app.Get("/v5/blocks/hash/:hash", func(c *fiber.Ctx) (err error) {
		if block, err := blk.BlockByHash(c.Context(), c.Params("hashOrHeight")); err != nil {
			return err
		} else if block == nil {
			return c.SendStatus(404)
		} else {
			c.Set("Cache-Control", "public,max-age=31536000,immutable")
			return c.JSON(block)
		}
	})

	app.Get("/v5/blocks/list/:from", func(c *fiber.Ctx) error {
		if fromHeight, err := strconv.ParseUint(c.Params("from"), 10, 32); err != nil {
			return c.SendStatus(400)
		} else if headers, err := blk.Blocks(c.Context(), fromHeight, 10000); err != nil {
			return err
		} else {
			return c.JSON(headers)
		}
	})

	app.Post("/v5/tx", func(c *fiber.Ctx) error {
		if tx, err := transaction.NewTransactionFromBytes(c.Body()); err != nil {
			return c.SendStatus(400)
		} else if txid, err := broadcast.Broadcast(c.Context(), tx); err != nil {
			return err
		} else {
			return c.SendString(txid)
		}

	})

	app.Get("/v5/tx/:txid", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if rawtx, err := jb.LoadRawtx(c.Context(), txid); err != nil {
			return err
		} else if len(rawtx) == 0 {
			return c.SendStatus(404)
		} else {
			c.Set("Content-Type", "application/octet-stream")
			buf := bytes.NewBuffer([]byte{})
			buf.Write(transaction.VarInt(uint64(len(rawtx))).Bytes())
			buf.Write(rawtx)
			if proof, err := jb.LoadProof(c.Context(), txid); err != nil {
				return err
			} else {
				if proof == nil {
					buf.Write(transaction.VarInt(0).Bytes())
					c.Set("Cache-Control", "public,max-age=60")
				} else {
					if proof.BlockHeight < chaintip.Height-5 {
						c.Set("Cache-Control", "public,max-age=31536000,immutable")
					} else {
						c.Set("Cache-Control", "public,max-age=60")
					}
					bin := proof.Bytes()
					buf.Write(transaction.VarInt(uint64(len(bin))).Bytes())
					buf.Write(bin)
				}
			}
			return c.Send(buf.Bytes())
		}
	})

	app.Get("/v5/tx/:txid/raw", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if rawtx, err := jb.LoadRawtx(c.Context(), txid); err != nil {
			return err
		} else if rawtx == nil {
			return c.SendStatus(404)
		} else {
			c.Set("Cache-Control", "public,max-age=31536000,immutable")
			c.Set("Content-Type", "application/octet-stream")
			return c.Send(rawtx)
		}
	})

	app.Get("/v5/tx/:txid/proof", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if proof, err := jb.LoadProof(c.Context(), txid); err != nil {
			return err
		} else if proof == nil {
			return c.SendStatus(404)
		} else {
			c.Set("Content-Type", "application/octet-stream")
			if proof.BlockHeight < chaintip.Height-5 {
				c.Set("Cache-Control", "public,max-age=31536000,immutable")
			} else {
				c.Set("Cache-Control", "public,max-age=60")
			}
			return c.Send(proof.Bytes())
		}
	})

	app.Get("/v5/txo/:outpoint", func(c *fiber.Ctx) error {
		if txo, err := idx.LoadTxo(c.Context(), c.Params("outpoint"), indexedTags); err != nil {
			return err
		} else if txo == nil {
			return c.SendStatus(404)
		} else {
			c.Set("Cache-Control", "public,max-age=60")
			return c.JSON(txo)
		}
	})

	app.Get("/v5/address/:address/utxos", func(c *fiber.Ctx) (err error) {
		address := c.Params("address")

		var tags []string
		if queryTags, ok := c.Queries()["tags"]; !ok {
			tags = indexedTags
		} else {
			tags = strings.Split(queryTags, ",")
		}

		if txos, err := idx.AddressUtxos(c.Context(), address, tags); err != nil {
			return err
		} else {
			return c.JSON(txos)
		}
	})

	// app.Get("/v5/address/:address/:from", func(c *fiber.Ctx) (err error) {
	// 	address := c.Params("address")
	// 	var start float64
	// 	if start, err = strconv.ParseFloat(c.Params("from"), 64); err != nil {
	// 		return err
	// 	}
	// 	scores := make([]float64, 0, 1000)
	// 	txMap := make(map[float64]*lib.TxResult)
	// 	if outpoints, err := data.Rdb.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
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

	app.Put("/v5/acct/:account", func(c *fiber.Ctx) error {
		account := c.Params("account")
		var owners []string
		if err := c.BodyParser(&owners); err != nil {
			return c.SendStatus(400)
		} else if len(owners) == 0 {
			return c.SendStatus(400)
		}

		if _, err := idx.UpdateAccount(c.Context(), account, owners); err != nil {
			return err
		} else if err := idx.SyncAcct(c.Context(), TAG, account, ingest); err != nil {
			return err
		}

		return c.SendStatus(204)
	})

	app.Get("/v5/acct/:account/utxos", func(c *fiber.Ctx) (err error) {
		account := c.Params("account")

		var tags []string
		if queryTags, ok := c.Queries()["tags"]; !ok {
			tags = indexedTags
		} else {
			tags = strings.Split(queryTags, ",")
		}

		if txos, err := idx.AccountUtxos(c.Context(), account, tags); err != nil {
			return err
		} else {
			return c.JSON(txos)
		}
	})

	app.Get("/v5/acct/:account/:from", func(c *fiber.Ctx) (err error) {
		account := c.Params("account")
		var start float64
		if start, err = strconv.ParseFloat(c.Params("from"), 64); err != nil {
			return err
		}

		results := make([]*lib.TxResult, 0, 1000)
		txMap := make(map[float64]*lib.TxResult)
		if outpoints, err := idx.TxoDB.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
			Key:     idx.AccountTxosKey(account),
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

	app.Get("/v5/sse", func(c *fiber.Ctx) error {
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
	// <-make(chan struct{})
}
