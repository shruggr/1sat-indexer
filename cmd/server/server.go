package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/ingest"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
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

func main() {
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")
	flag.Parse()

	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())

	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("Yo!")
	})
	app.Get("/ord/:address", func(c *fiber.Ctx) error {
		address := c.Params("address")
		err := ordinals.RefreshAddress(ctx, address)
		if err != nil {
			log.Println("RefreshAddress", err)
			return err
		}
		c.SendStatus(http.StatusNoContent)
		return nil
	})

	app.Get("/origin/:origin/latest", func(c *fiber.Ctx) error {
		origin, err := lib.NewOutpointFromString(c.Params("origin"))
		if err != nil {
			log.Println("Parse origin", err)
			return err
		}

		outpoint, err := ordinals.GetLatestOutpoint(ctx, origin)
		if err != nil {
			log.Println("GetLatestOutpoint", err)
			return err
		}
		return c.Send(*outpoint)
	})

	app.Get("/tx/:txid/ingest", func(c *fiber.Ctx) error {
		txid := c.Params("txid")
		if _, err := ingest.IngestTxid(txid); err != nil {
			log.Println("IngestTxid", err)
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})

	// app.Get("/inscription/:outpoint/index", func(c *fiber.Ctx) error {
	// 	// cacheKey := "origin:" + c.Params("origin")
	// 	// cached, err := rdb.Get(ctx, cacheKey).Bytes()
	// 	// if err == nil && len(cached) > 0 {
	// 	// 	return c.Send(cached)
	// 	// }

	// 	outpoint, err := lib.NewOutpointFromString(c.Params("outpoint"))
	// 	if err != nil {
	// 		log.Println("Parse origin", err)
	// 		return err
	// 	}

	// 	// outpoint, err := ordinals.GetLatestOutpoint(ctx, origin)
	// 	// if err != nil {
	// 	// 	log.Println("GetLatestOutpoint", err)
	// 	// 	return err
	// 	// }
	// 	// rdb.SetEx(ctx, cacheKey, *outpoint, 3*time.Second)
	// 	return c.Send(*outpoint)
	// })

	// Temporary fix for unfound memory leak
	go func() {
		log.Println("Registering Reset Timer")
		resetTimer := time.NewTimer(30 * time.Minute)
		<-resetTimer.C
		log.Println("Resetting")
		os.Exit(0)
	}()
	log.Println("Listening on", PORT)
	app.Listen(fmt.Sprintf(":%d", PORT))
}
