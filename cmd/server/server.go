package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/cmd/server/sse"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/valyala/fasthttp"
)

var POSTGRES string
var CONCURRENCY int
var PORT int

var ctx = context.Background()
var jb *junglebus.Client

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}

	log.Println("POSTGRES:", POSTGRES)
	var err error
	config, err := pgxpool.ParseConfig(POSTGRES)
	if err != nil {
		log.Panic(err)
	}
	config.MaxConnIdleTime = 15 * time.Second

	db, err := pgxpool.NewWithConfig(context.Background(), config)
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

	JUNGLEBUS := os.Getenv("JUNGLEBUS")
	if JUNGLEBUS == "" {
		JUNGLEBUS = "https://junglebus.gorillapool.io"
	}

	jb, err = junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panicln(err.Error())
	}

}

var currentSessions = sse.SessionsLock{
	Topics: make(map[string][]*sse.Session),
}

func main() {
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.Parse()

	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())

	app.Get("/v1/blocks/tip", func(c *fiber.Ctx) error {
		if tip, err := lib.Rdb.Get(ctx, lib.ChaintipKey).Result(); err != nil {
			return err
		} else {
			c.Set("Content-Type", "application/json")
			return c.SendString(tip)
		}
	})

	app.Get("/v1/blocks/list/:from", func(c *fiber.Ctx) error {
		if height, err := strconv.ParseInt(c.Params("from"), 10, 32); err != nil {
			return c.SendStatus(400)
		} else if blockJsons, err := lib.Rdb.LRange(c.Context(), lib.BlocksKey, height, height+10000).Result(); err != nil {
			return err
		} else {
			blocks := make([]*models.BlockHeader, 0, len(blockJsons))
			for _, blockJson := range blockJsons {
				var block models.BlockHeader
				if err := json.Unmarshal([]byte(blockJson), &block); err != nil {
					return err
				}
				blocks = append(blocks, &block)
			}

			return c.JSON(blocks)
		}
	})

	app.Get("/v1/address/:address/:from", func(c *fiber.Ctx) (err error) {
		address := c.Params("address")
		var start float64
		if start, err = strconv.ParseFloat(c.Params("from"), 64); err != nil {
			return err
		}

		scores := make([]float64, 0, 250)
		txMap := make(map[float64]*lib.TxResult)
		if outpoints, err := lib.Rdb.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
			Key:     lib.OwnerTxosKey(address),
			ByScore: true,
			Start:   start,
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
					height := uint32(item.Score)
					result = &lib.TxResult{
						Txid:    txid,
						Height:  height,
						Idx:     uint64((item.Score - float64(height)) * 1000000000),
						Outputs: lib.NewOutputMap(),
					}

					txMap[item.Score] = result
					scores = append(scores, item.Score)
				}
				if out != nil {
					result.Outputs[*out] = struct{}{}
				}
			}
			slices.Sort(scores)
			results := make([]*lib.TxResult, 0, len(scores))
			for _, score := range scores {
				results = append(results, txMap[score])
			}
			return c.JSON(results)
		}
	})

	app.Get("/v1/acct/:account/:from", func(c *fiber.Ctx) (err error) {
		account := c.Params("account")
		var start float64
		if start, err = strconv.ParseFloat(c.Params("from"), 64); err != nil {
			return err
		}

		scores := make([]float64, 0, 250)
		txMap := make(map[float64]*lib.TxResult)
		if outpoints, err := lib.Rdb.ZRangeArgsWithScores(c.Context(), redis.ZRangeArgs{
			Key:     lib.AccountTxosKey(account),
			ByScore: true,
			Start:   start,
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
					height := uint32(item.Score)
					result = &lib.TxResult{
						Txid:    txid,
						Height:  height,
						Idx:     uint64((item.Score - float64(height)) * 1000000000),
						Outputs: lib.NewOutputMap(),
					}

					txMap[item.Score] = result
					scores = append(scores, item.Score)
				}
				if out != nil {
					result.Outputs[*out] = struct{}{}
				}
			}
		}
		slices.Sort(scores)
		results := make([]*lib.TxResult, 0, len(scores))
		for _, score := range scores {
			results = append(results, txMap[score])
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
		for _, owner := range owners {
			if owner == "" {
				return c.SendStatus(400)
			}
			ownerTxoKeys = append(ownerTxoKeys, lib.OwnerTxosKey(owner))
		}
		if _, err := lib.Rdb.Pipelined(c.Context(), func(pipe redis.Pipeliner) error {
			for _, owner := range owners {
				if err := pipe.SAdd(c.Context(), lib.AccountKey(account), owner).Err(); err != nil {
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
		return c.SendStatus(204)
	})

	// app.Get("/v1/sse", func(c *fiber.Ctx) error {
	// 	c.Set("Content-Type", "text/event-stream")
	// 	c.Set("Cache-Control", "no-cache")
	// 	c.Set("Connection", "keep-alive")
	// 	c.Set("Transfer-Encoding", "chunked")

	// 	topicVal := c.Queries()["topic"]
	// 	topics := strings.Split(topicVal, ",")
	// 	if len(topics) == 0 {
	// 		return c.SendStatus(400)
	// 	}
	// 	log.Println("Subscribing to", topics)
	// 	interval := time.NewTicker(15 * time.Second)
	// 	sub := redis.NewClient(&redis.Options{
	// 		Addr:     os.Getenv("REDISDB"),
	// 		Password: "", // no password set
	// 		DB:       0,  // use default DB
	// 	})
	// 	defer func() {
	// 		interval.Stop()
	// 		sub.Close()
	// 	}()

	// 	ch := sub.Subscribe(context.Background(), topics...).Channel()
	// 	notify := c.Context().Done()
	// 	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
	// 		for loop := true; loop; {
	// 			select {
	// 			case <-notify:
	// 				return
	// 			case <-interval.C:
	// 				c.WriteString(":keepalive\n")
	// 				log.Println("Sending keepalive")
	// 			case msg := <-ch:
	// 				if _, err := fmt.Fprintf(w, fmt.Sprintf("event: %s\ndata: %s\n\n", msg.Channel, msg.Payload)); err != nil {
	// 					log.Printf("Error while writing Data: %v\n", err)
	// 					loop = false
	// 				}

	// 				log.Println("Sending Event", msg.Channel, msg.Payload)
	// 			}
	// 		}

	// 		log.Println("Exiting stream")
	// 	}))
	// 	return nil
	// })

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

		notify := c.Context().Done()

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			keepAliveTickler := time.NewTicker(15 * time.Second)
			keepAliveMsg := ":keepalive\n"

			// listen to signal to close and unregister (doesn't seem to be called)
			go func() {
				<-notify
				log.Printf("Stopped Request\n")
				currentSessions.RemoveSession(&s)
				keepAliveTickler.Stop()
			}()

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
					_, err = fmt.Fprintf(w, sseMessage)

					if err != nil {
						log.Printf("Error while writing Data: %v\n", err)
						continue
					}

					err = w.Flush()
					if err != nil {
						log.Printf("Error while flushing Data: %v\n", err)
						currentSessions.RemoveSession(&s)
						keepAliveTickler.Stop()
						loop = false
						break
					}
				case <-keepAliveTickler.C:
					fmt.Fprintf(w, keepAliveMsg)
					err := w.Flush()
					if err != nil {
						log.Printf("Error while flushing: %v.\n", err)
						currentSessions.RemoveSession(&s)
						keepAliveTickler.Stop()
						loop = false
						break
					}
				}
			}

			log.Println("Exiting stream")
		}))

		return nil
	})

	// ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	// defer cancel()

	// ticker := time.NewTicker(1 * time.Second)

	go func() {
		sub := redis.NewClient(&redis.Options{
			Addr:     os.Getenv("REDISCACHE"),
			Password: "", // no password set
			DB:       0,  // use default DB
		})
		ctx := context.Background()
		pubsub := sub.Subscribe(ctx, "act:shruggr")
		ch := pubsub.Channel()
		iterval := time.NewTicker(5 * time.Second)

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
			case <-iterval.C:
				if err := lib.Cache.Publish(ctx, "act:shruggr", "test"); err != nil {
					log.Println("Error publishing", err)
				}
				log.Println("Publishing")
			case <-ctx.Done():
				return
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
