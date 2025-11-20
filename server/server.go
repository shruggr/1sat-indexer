package server

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/broadcast"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/server/routes/acct"
	"github.com/shruggr/1sat-indexer/v5/server/routes/blocks"
	"github.com/shruggr/1sat-indexer/v5/server/routes/evt"
	"github.com/shruggr/1sat-indexer/v5/server/routes/origins"
	"github.com/shruggr/1sat-indexer/v5/server/routes/own"
	"github.com/shruggr/1sat-indexer/v5/server/routes/spend"
	"github.com/shruggr/1sat-indexer/v5/server/routes/sse"
	"github.com/shruggr/1sat-indexer/v5/server/routes/tag"
	"github.com/shruggr/1sat-indexer/v5/server/routes/tx"
	"github.com/shruggr/1sat-indexer/v5/server/routes/txos"
)

var currentSessions = sse.SessionsLock{
	Topics: make(map[string][]*sse.Session),
}

func Initialize(ingestCtx *idx.IngestCtx, arcBroadcaster *broadcaster.Arc) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit: 100 * 1024 * 1024, // 100MB
	})
	// app.Use(recover.New())
	app.Use(logger.New())
	app.Use(compress.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*"}))

	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("yo")
	})

	v5 := app.Group("/v5")

	acct.RegisterRoutes(v5.Group("/acct"), ingestCtx)
	blocks.RegisterRoutes(v5.Group("/blocks"))
	evt.RegisterRoutes(v5.Group("/evt"), ingestCtx)
	origins.RegisterRoutes(v5.Group("/origins"), ingestCtx)
	own.RegisterRoutes(v5.Group("/own"), ingestCtx)
	tag.RegisterRoutes(v5.Group("/tag"), ingestCtx)
	tx.RegisterRoutes(v5.Group("/tx"), ingestCtx, arcBroadcaster)
	txos.RegisterRoutes(v5.Group("/txo"), ingestCtx)
	spend.RegisterRoutes(v5.Group("/spends"), ingestCtx)

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

		// Get starting score from Last-Event-ID header or from query param
		fromScore := -1.0
		lastEventID := c.Get("Last-Event-ID")
		if lastEventID != "" {
			if parsed, err := strconv.ParseFloat(lastEventID, 64); err == nil {
				fromScore = parsed
			}
		} else if fromParam := c.Query("from"); fromParam != "" {
			if parsed, err := strconv.ParseFloat(fromParam, 64); err == nil {
				fromScore = parsed
			}
		}

		log.Println("Subscribing to", topics, "from score", fromScore)

		// Perform catchup query for historical events since fromScore
		if fromScore >= 0 {
			ctx := context.Background()
			for _, topic := range topics {
				catchupTxos, err := ingestCtx.Store.SearchTxos(ctx, &idx.SearchCfg{
					Keys:    []string{topic},
					From:    &fromScore,
					Limit:   1000,
					Reverse: false,
				})
				if err != nil {
					log.Printf("Error fetching catchup txos for topic %s: %v\n", topic, err)
					continue
				}

				// Send catchup events
				for _, txo := range catchupTxos {
					sseMessage, err := sse.FormatSSEMessage(topic, txo, txo.Score)
					if err != nil {
						log.Printf("Error formatting catchup message: %v\n", err)
						continue
					}
					if _, err := c.WriteString(sseMessage); err != nil {
						log.Printf("Error writing catchup message: %v\n", err)
						return err
					}
				}
			}
		}

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

				// Parse the payload to extract score
				var txo idx.Txo
				if err := json.Unmarshal([]byte(ev.Payload), &txo); err != nil {
					log.Printf("Error unmarshaling txo: %v\n", err)
					continue
				}

				sseMessage, err := sse.FormatSSEMessage(ev.Channel, ev.Payload, txo.Score)
				if err != nil {
					log.Printf("Error formatting sse message: %v\n", err)
					continue
				}

				// send sse formatted message
				if _, err := c.WriteString(sseMessage); err != nil {
					log.Printf("Error while writing message: %v\n", err)
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

	// SSE listener for block and other events
	go func() {
		if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
			panic(err)
		} else {
			sub := redis.NewClient(opts)
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
					log.Println("Received Message", msg.Channel)
					for _, session := range currentSessions.Topics[msg.Channel] {
						session.StateChannel <- msg
					}
				}
			}
		}
	}()

	// Broadcast status listener for ARC callbacks
	go func() {
		if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
			log.Printf("Error parsing REDISEVT URL for broadcast listener: %v", err)
			return
		} else {
			statusRedis := redis.NewClient(opts)
			listener := broadcast.InitListener(statusRedis)
			ctx := context.Background()
			listener.Start(ctx)
		}
	}()

	return app
}
