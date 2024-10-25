package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/blk"
	"github.com/shruggr/1sat-indexer/cmd/server/sse"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
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

var ingest *idx.IngestCtx

var chaintip *blk.BlockHeader

func init() {
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.StringVar(&TAG, "tag", "ingest", "Ingest tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	ingest = &idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
	}

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
	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(compress.New())

	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("yo")
	})

	v5 := app.Group("/v5")

	RegisterBlockRoutes(v5.Group("/blocks"))
	RegisterTxRoutes(v5.Group("/tx"))

	app.Get("/v5/txo/tag/:tag", func(c *fiber.Ctx) error {
		var tags []string
		if dataTags, ok := c.Queries()["tags"]; !ok {
			tags = indexedTags
		} else {
			tags = strings.Split(dataTags, ",")
		}
		from, err := strconv.ParseFloat(c.Params("from", "0"), 64)
		if err != nil {
			return err
		}

		if outpoints, err := idx.SearchUtxos(c.Context(), &idx.SearchCfg{
			Key:     evt.TagKey(c.Params("tag")),
			From:    from,
			Reverse: c.QueryBool("rev", false),
		}); err != nil {
			return err
		} else if txos, err := idx.LoadTxos(c.Context(), outpoints, tags); err != nil {
			return err
		} else {
			return c.JSON(txos)
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
		if dataTags, ok := c.Queries()["tags"]; !ok {
			tags = indexedTags
		} else {
			tags = strings.Split(dataTags, ",")
		}

		if outpoints, err := idx.SearchUtxos(c.Context(), &idx.SearchCfg{
			Key: idx.OwnerTxosKey(address),
		}); err != nil {
			return err
		} else if txos, err := idx.LoadTxos(c.Context(), outpoints, tags); err != nil {
			return err
		} else {
			return c.JSON(txos)
		}
	})

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

		if outpoints, err := idx.SearchUtxos(c.Context(), &idx.SearchCfg{
			Key: idx.AccountTxosKey(account),
		}); err != nil {
			return err
		} else if txos, err := idx.LoadTxos(c.Context(), outpoints, tags); err != nil {
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
