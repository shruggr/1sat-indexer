package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
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

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}

	log.Println("POSTGRES:", POSTGRES)
	var err error
	db, err = pgxpool.New(context.Background(), POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	ordinals.Initialize(db, rdb)
}

func main() {
	// flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&PORT, "p", 8081, "Port to listen on")

	app := fiber.New()
	app.Use(recover.New())

	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("Yo!")
	})
	app.Get("/ord/:address", func(c *fiber.Ctx) error {
		address := c.Params("address")
		row := db.QueryRow(c.Context(),
			"SELECT height FROM addresses WHERE address=$1",
			address,
		)
		var height uint32
		row.Scan(&height)

		url := fmt.Sprintf("%s/v1/address/get/%s/%d", os.Getenv("JUNGLEBUS"), address, height)
		log.Println("URL:", url)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		txns := []*AddressTxn{}
		err = json.NewDecoder(resp.Body).Decode(&txns)
		if err != nil {
			return err
		}

		txids := make([][]byte, len(txns))
		toIndex := map[string]*AddressTxn{}
		for i, txn := range txns {
			txids[i] = txn.Txid
			toIndex[txn.Txid.String()] = txn
			if txn.Height > height {
				height = txn.Height
			}
		}

		rows, err := db.Query(c.Context(), `
			SELECT encode(txid, 'hex')
			FROM txn_indexer 
			WHERE indexer='ord' AND txid = ANY($1)`,
			txids,
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
		var wg sync.WaitGroup
		for txid, txn := range toIndex {
			wg.Add(1)
			go func(txid string, txn *AddressTxn) {
				defer wg.Done()
				if rawtx, err := lib.LoadRawtx(txid); err == nil {
					ordinals.IndexTxn(rawtx, txn.BlockId, txn.Height, txn.Idx, false)
				}
			}(txid, txn)
		}
		wg.Wait()
		if height == 0 {
			height = 817000
		}
		_, err = db.Exec(c.Context(), `
			INSERT INTO addresses(address, height)
			VALUES ($1, $2) 
			ON CONFLICT (address) DO UPDATE SET 
				height = $2, updated = CURRENT_TIMESTAMP`,
			address,
			height,
		)
		if err != nil {
			log.Panicf("Error updating address: %s", err)
		}
		c.SendStatus(http.StatusNoContent)
		return nil
	})

	app.Listen(fmt.Sprintf(":%d", PORT))
}
