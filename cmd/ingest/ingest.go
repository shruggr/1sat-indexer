package main

import (
	"context"
	"flag"
	"log"

	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var CONCURRENCY uint
var VERBOSE int
var QUEUE string
var TAG string

// var sub *redis.Client

func init() {
	flag.StringVar(&TAG, "tag", idx.IngestTag, "Log tag")
	flag.StringVar(&QUEUE, "q", idx.IngestTag, "Queue tag")
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	// if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
	// 	panic(err)
	// } else {
	// 	sub = redis.NewClient(opts)
	// }
}

func main() {
	ctx := context.Background()
	ingest := &idx.IngestCtx{
		Tag:         TAG,
		Key:         idx.QueueKey(QUEUE),
		Indexers:    config.Indexers,
		Network:     config.Network,
		Concurrency: CONCURRENCY,
		Once:        true,
		// Verbose:     VERBOSE > 0,
		Verbose:  true,
		Store:    config.Store,
		PageSize: 100000,
	}

	// ch := sub.Subscribe(context.Background(), "submit", "broadcast").Channel()
	// go func() {
	// 	log.Println("Listening for events")
	// 	for msg := range ch {
	// 		log.Println("Event", msg.Channel, msg.Payload)
	// 		switch msg.Channel {
	// 		case "submit":
	// 			log.Println("[SUBMIT]")
	// 			go func(txid string) {
	// 				defer func() {
	// 					if r := recover(); r != nil {
	// 						fmt.Println("Recovered in submit", r)
	// 					}
	// 				}()
	// 				for i := 0; i < 4; i++ {
	// 					tx, err := jb.LoadTx(ctx, txid, true)
	// 					if err == nil {
	// 						ingest.IngestTx(ctx, tx, idx.AncestorConfig{Load: true, Parse: true, Save: true})
	// 					}
	// 					log.Printf("[RETRY] %d: %s\n", i, txid)
	// 					switch i {
	// 					case 0:
	// 						time.Sleep(2 * time.Second)
	// 					case 1:
	// 						time.Sleep(10 * time.Second)
	// 					default:
	// 						time.Sleep(30 * time.Second)
	// 					}
	// 				}
	// 			}(msg.Payload)
	// 		case "broadcast":
	// 			log.Println("[BROADCAST]")
	// 			rawtx, err := base64.StdEncoding.DecodeString(msg.Payload)
	// 			if err != nil {
	// 				continue
	// 			}
	// 			go func(rawtx []byte) {
	// 				defer func() {
	// 					if r := recover(); r != nil {
	// 						fmt.Println("Recovered in broadcast")
	// 					}
	// 				}()
	// 				if tx, err := transaction.NewTransactionFromBytes(rawtx); err != nil {
	// 					log.Println("Broadcast event error", err)
	// 				} else if _, err := ingest.IngestTx(ctx, tx, idx.AncestorConfig{Load: true, Parse: true, Save: false}); err != nil {
	// 					log.Println("Broadcast event ingest error", err)
	// 				}
	// 			}(rawtx)
	// 		}
	// 	}
	// }()
	if err := (ingest).Exec(ctx); err != nil {
		log.Println("Ingest error", err)
	}
}
