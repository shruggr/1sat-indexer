package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "node"

// var THREADS uint64 = 16

var db *sql.DB
var rdb *redis.Client

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err)
	}
}

var ch *amqp.Channel
var txnQ amqp.Queue
var blockQ amqp.Queue

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}
	db.SetConnMaxIdleTime(time.Millisecond * 100)
	db.SetMaxOpenConns(400)
	db.SetMaxIdleConns(25)

	fmt.Println("REDIS:", os.Getenv("REDIS"))
	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	rmq, err := amqp.Dial(os.Getenv("RABBITMQ"))
	failOnError(err, "Failed to connect to RabbitMQ")

	ch, err = rmq.Channel()
	failOnError(err, "Failed to open a channel")

	// txnQ, err = ch.QueueDeclare(
	// 	"txns", // name
	// 	false,  // durable
	// 	true,   // delete when unused
	// 	false,  // exclusive
	// 	false,  // no-wait
	// 	amqp.Table{"x-consumer-timeout": 10000},
	// )
	// failOnError(err, "Failed to declare a queue")

	err = ch.ExchangeDeclare(
		"blocks", // name
		"fanout", // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	failOnError(err, "Failed to declare an exchange")

	blockQ, err = ch.QueueDeclare(
		"blocks", // name
		true,     // durable
		false,    // delete when unused
		false,    // exclusive
		false,    // no-wait
		nil,      // arguments
	)
	failOnError(err, "Failed to declare a queue")

	err = ch.QueueBind(
		blockQ.Name, // queue name
		"",          // routing key
		"blocks",    // exchange
		false,
		nil,
	)
	failOnError(err, "Failed to bind a queue")

	txnQ, err = ch.QueueDeclare(
		"txns", // name
		false,  // durable
		false,  // delete when unused
		false,  // exclusive
		false,  // no-wait
		amqp.Table{"x-consumer-timeout": 10000},
	)
	failOnError(err, "Failed to declare a queue")
}

var queryCount uint64

func main() {
	txnMsgs, err := ch.Consume(
		"txns", // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Failed to register a consumer")

	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for t := range ticker.C {
			// select {
			// case t := <-ticker.C:
			log.Println(queryCount, "queries in 10 seconds", t.Format(time.RFC3339))
			queryCount = 0
			// }
		}
	}()

	forever := make(chan bool)
	go func() {
		for d := range txnMsgs {
			// var txn lib.TxnTask
			// dec := gob.NewDecoder(bytes.NewReader(d.Body))
			// err := dec.Decode(&txn)
			// log.Printf("Received a message: %s", d.Body)
			txn, err := lib.NewTxnFromBytes(d.Body)
			failOnError(err, "Failed to decode txn")
			// log.Printf("Received a message: %v", txn)
			result, err := txn.Index(false)
			failOnError(err, "Failed to index txn "+txn.HexId)
			queryCount += result.QueryCount

			err = ch.PublishWithContext(
				context.Background(),
				"",
				d.ReplyTo,
				true,
				false,
				amqp.Publishing{
					ContentType:   "text/plain",
					CorrelationId: d.CorrelationId,
					Body:          binary.BigEndian.AppendUint64([]byte{}, result.Fees),
				},
			)
			failOnError(err, "Failed to publish a message")

			err = d.Ack(false)
			failOnError(err, "Failed to ack message")
		}
		fmt.Println("Exiting consumer")
	}()
	<-forever
}
