package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/ordishs/go-bitcoin"
)

const INDEXER = "node"
const GENESIS = "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"

var THREADS uint64 = 8

var db *sql.DB

var httpClient = &http.Client{Timeout: 10 * time.Minute}

// var q amqp.Queue
// var ch *amqp.Channel

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	failOnError(err, "Failed to connect to Postgres")

	db.SetConnMaxIdleTime(time.Millisecond * 100)
	db.SetMaxOpenConns(400)
	db.SetMaxIdleConns(25)

	// rmq, err := amqp.Dial(os.Getenv("RABBITMQ"))
	// failOnError(err, "Failed to connect to RabbitMQ")

	// ch, err = rmq.Channel()
	// failOnError(err, "Failed to open a channel")

	// q, err = ch.QueueDeclare(
	// 	"blocks", // name
	// 	true,     // durable
	// 	false,    // delete when unused
	// 	false,    // exclusive
	// 	false,    // noWait
	// 	nil,      // arguments
	// )
	// failOnError(err, "Failed to declare a queue")
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

func main() {
	var hash string
	var height uint32
	var accSub uint64
	row := db.QueryRow(`SELECT encode(id, 'hex'), height, subacc
		FROM blocks
		ORDER BY height DESC
		LIMIT 1`,
	)
	row.Scan(&hash, &height, &accSub)
	if hash == "" {
		hash = GENESIS
	}
	fmt.Printf("fromBlock %d %s", height, hash)

	for {
		headers := func() (headers []*bitcoin.BlockHeader) {
			url := fmt.Sprintf("https://ordinals.shruggr.cloud/rest/headers/1000/%s.json", hash)
			fmt.Println("Headers", url)
			r, err := httpClient.Get(url)
			if err != nil {
				log.Panic(err)
			}
			defer r.Body.Close()

			headers = []*bitcoin.BlockHeader{}
			// header := []bitcoin.BlockHeader{}
			// header := wire.BlockHeader{}
			// protocolVersion := wire.ProtocolVersion
			// err = header.Bsvdecode(r.Body, protocolVersion, wire.BaseEncoding)
			// props := map[string]interface{}{}
			err = json.NewDecoder(r.Body).Decode(&headers)
			if err != nil {
				panic(err)
			}
			return
		}()

		if len(headers) == 0 {
			log.Println("Waiting for block to be mined")
			time.Sleep(time.Minute)
			continue
		}
		buf := make([]byte, 1024*1024)
		for _, header := range headers[1:] {
			func() {
				fmt.Println("Downloading", header.Height, hash)
				r, err := httpClient.Get(fmt.Sprintf("https://ordinals.shruggr.cloud/rest/block/%s.bin", hash))
				if err != nil {
					log.Panic(err)
				}
				defer r.Body.Close()
				f, err := os.Create(fmt.Sprintf("/home/shruggr/blocks/%s.bin", hash))
				if err != nil {
					log.Panic(err)
				}
				w := bufio.NewWriter(f)
				defer f.Close()

				for {
					// read a chunk
					n, err := r.Body.Read(buf)
					if err != nil && err != io.EOF {
						panic(err)
					}
					if n == 0 {
						break
					}

					// write a chunk
					if _, err := w.Write(buf[:n]); err != nil {
						panic(err)
					}
				}

				if err = w.Flush(); err != nil {
					panic(err)
				}

				subsidy := uint64(5000000000)
				halvings := uint8(header.Height / 210000)
				for i := 0; i < int(halvings); i++ {
					subsidy /= 2
				}
				accSub += subsidy
				if _, err = db.Exec(`INSERT INTO blocks (id, height, subsidy, subacc)
					VALUES (decode($1, 'hex'), $2, $3, $4)
					ON CONFLICT (id) DO NOTHING`,
					hash,
					header.Height,
					subsidy,
					accSub,
				); err != nil {
					panic(err)
				}

				// body, err := json.Marshal(header)
				// failOnError(err, "Failed to marshal header")
				// err = ch.PublishWithContext(context.Background(),
				// 	"",     // exchange
				// 	q.Name, // routing key
				// 	false,  // mandatory
				// 	false,  // immediate
				// 	amqp.Publishing{
				// 		ContentType: "application/json",
				// 		Body:        body,
				// 	})
				// failOnError(err, "Failed to publish a message")
			}()
			hash = header.Hash
			if hash == "" {
				log.Println("Waiting for block to be mined")
				time.Sleep(time.Minute)
				break
			}
		}
	}
}
