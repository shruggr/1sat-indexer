package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

var POSTGRES string
var CONCURRENCY int
var PORT int
var db *pgxpool.Pool
var rdb *redis.Client
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

	db, err = pgxpool.NewWithConfig(context.Background(), config)

	// db, err = pgxpool.New(context.Background(), POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

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

	ordinals.Initialize(db, rdb)
}

func main() {
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&PORT, "p", 8082, "Port to listen on")

	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())

	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("Yo!")
	})
	app.Get("/ord/:address", func(c *fiber.Ctx) error {
		address := c.Params("address")
		row := db.QueryRow(c.Context(),
			"SELECT height, updated FROM addresses WHERE address=$1",
			address,
		)
		var height uint32
		var updated time.Time
		row.Scan(&height, &updated)

		if time.Since(updated) < 30*time.Minute {
			log.Println("Frequent Update", address)
		}
		url := fmt.Sprintf("%s/v1/address/get/%s/%d", os.Getenv("JUNGLEBUS"), address, height)
		// log.Println("URL:", url)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		txns := []*AddressTxn{}
		err = json.NewDecoder(resp.Body).Decode(&txns)
		if err != nil {
			return err
		}

		// txids := make([][]byte, len(txns))
		toIndex := map[string]*AddressTxn{}
		batches := [][][]byte{}
		batch := make([][]byte, 0, 100)
		// log.Println("Txns:", len(txns))
		for i, txn := range txns {
			batch = append(batch, txn.Txid)
			toIndex[txn.Txid.String()] = txn
			if txn.Height > height {
				height = txn.Height
			}

			if i%100 == 99 {
				batches = append(batches, batch)
				batch = make([][]byte, 0, 100)
			}
		}
		if len(txns)%100 != 99 {
			batches = append(batches, batch)
		}

		for _, batch := range batches {
			// log.Println("Batch", len(batch))
			if len(batch) == 0 {
				break
			}
			rows, err := db.Query(c.Context(), `
				SELECT encode(txid, 'hex')
				FROM txn_indexer 
				WHERE indexer='ord' AND txid = ANY($1)`,
				batch,
			)
			if err != nil {
				log.Println(err)
				return c.SendStatus(http.StatusInternalServerError)
			}
			defer rows.Close()

			for rows.Next() {
				var txid string
				err := rows.Scan(&txid)
				if err != nil {
					return err
				}
				delete(toIndex, txid)
			}
			rows.Close()
		}
		var wg sync.WaitGroup
		limiter := make(chan struct{}, 32)
		for txid, txn := range toIndex {
			wg.Add(1)
			limiter <- struct{}{}
			go func(txid string, txn *AddressTxn) {
				defer func() {
					wg.Done()
					<-limiter
				}()
				if rawtx, err := lib.LoadRawtx(txid); err == nil {
					// lib.IndexTxn(rawtx, txn.BlockId, txn.Height, txn.Idx, false)
					ordinals.IndexTxn(rawtx, txn.BlockId, txn.Height, txn.Idx)
				}
			}(txid, txn)
		}
		wg.Wait()
		if height == 0 {
			height = 817000
		}
		_, err = db.Exec(c.Context(), `
			INSERT INTO addresses(address, height, updated)
			VALUES ($1, $2, CURRENT_TIMESTAMP) 
			ON CONFLICT (address) DO UPDATE SET 
				height = EXCLUDED.height, 
				updated = CURRENT_TIMESTAMP`,
			address,
			height,
		)
		if err != nil {
			log.Panicf("Error updating address: %s", err)
		}
		// fmt.Println("Finished", address)
		c.SendStatus(http.StatusNoContent)
		return nil
	})

	app.Get("/origin/:origin/latest", func(c *fiber.Ctx) error {
		origin, err := lib.NewOutpointFromString(c.Params("origin"))
		if err != nil {
			return err
		}

		var op lib.Outpoint
		for {
			row := db.QueryRow(ctx, `
				SELECT outpoint, height
				FROM txos
				WHERE origin = $1
				ORDER BY CASE WHEN spend='\x' THEN 1 ELSE 0 END DESC, height DESC, idx DESC
				LIMIT 1`,
				origin,
			)
			var outpoint []byte
			var height sql.NullInt32
			err = row.Scan(&outpoint, &height)
			if err != nil {
				if err == pgx.ErrNoRows {
					return c.SendStatus(404)
				}
				return err
			}
			if bytes.Equal(op, outpoint) {
				// log.Printf("OPs Equal: %x=%x\n", op, outpoint)
				return c.Send(outpoint)
			}

			op = lib.Outpoint(outpoint)

			if !height.Valid || height.Int32 == 0 {
				txn, err := jb.GetTransaction(c.Context(), hex.EncodeToString(op.Txid()))
				// rawtx, err := lib.LoadRawtx(hex.EncodeToString(spend))
				if err != nil {
					return err
				}
				// log.Printf("Indexing: %s\n", hex.EncodeToString(spend))
				ordinals.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
			}
			url := fmt.Sprintf("%s/v1/txo/spend/%s", os.Getenv("JUNGLEBUS"), op.String())
			// log.Println("URL:", url)
			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			spend, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if len(spend) == 0 {
				// log.Printf("Empty Spend: %s\n", op.String())
				return c.Send(outpoint)
			}
			txn, err := jb.GetTransaction(c.Context(), hex.EncodeToString(spend))
			// rawtx, err := lib.LoadRawtx(hex.EncodeToString(spend))
			if err != nil {
				return err
			}
			// log.Printf("Indexing: %s\n", hex.EncodeToString(spend))
			ordinals.IndexTxn(txn.Transaction, txn.BlockHash, txn.BlockHeight, txn.BlockIndex)
		}
	})

	app.Listen(fmt.Sprintf(":%d", PORT))
}
