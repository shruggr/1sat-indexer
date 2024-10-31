package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/cmd/server/acct"
	"github.com/shruggr/1sat-indexer/cmd/server/blocks"
	"github.com/shruggr/1sat-indexer/cmd/server/evt"
	"github.com/shruggr/1sat-indexer/cmd/server/origins"
	"github.com/shruggr/1sat-indexer/cmd/server/own"
	"github.com/shruggr/1sat-indexer/cmd/server/sse"
	"github.com/shruggr/1sat-indexer/cmd/server/tag"
	"github.com/shruggr/1sat-indexer/cmd/server/tx"
	"github.com/shruggr/1sat-indexer/cmd/server/txos"
)

var PORT int

var currentSessions = sse.SessionsLock{
	Topics: make(map[string][]*sse.Session),
}

func init() {
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.Parse()
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

	acct.RegisterRoutes(v5.Group("/acct"))
	blocks.RegisterRoutes(v5.Group("/blocks"))
	evt.RegisterRoutes(v5.Group("/evt"))
	origins.RegisterRoutes(v5.Group("/origins"))
	own.RegisterRoutes(v5.Group("/own"))
	tag.RegisterRoutes(v5.Group("/tag"))
	tx.RegisterRoutes(v5.Group("/tx"))
	txos.RegisterRoutes(v5.Group("/txo"))

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
