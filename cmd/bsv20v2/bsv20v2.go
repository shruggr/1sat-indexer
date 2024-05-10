package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

// var settled = make(chan uint32, 1000)
var POSTGRES string
var db *pgxpool.Pool
var rdb *redis.Client
var INDEXER string
var TOPIC string
var FROM_BLOCK uint
var VERBOSE int
var CONCURRENCY int
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	flag.StringVar(&INDEXER, "id", "inscriptions", "Indexer name")
	flag.StringVar(&TOPIC, "t", "", "Junglebus SubscriptionID")
	flag.UintVar(&FROM_BLOCK, "s", uint(lib.TRIGGER), "Start from block")
	flag.IntVar(&CONCURRENCY, "c", 64, "Concurrency Limit")
	flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	flag.Parse()

	if POSTGRES == "" {
		POSTGRES = os.Getenv("POSTGRES_FULL")
	}
	var err error
	log.Println("POSTGRES:", POSTGRES)
	db, err = pgxpool.New(ctx, POSTGRES)
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = indexer.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	err = ordinals.Initialize(indexer.Db, indexer.Rdb)
	if err != nil {
		log.Panic(err)
	}
}

var pkhashFunds = map[string]*ordinals.V2TokenFunds{}
var idFunds = map[string]*ordinals.V2TokenFunds{}
var m sync.Mutex
var limiter chan struct{}
var sub *redis.PubSub

var currentHeight uint32

func main() {
	limiter = make(chan struct{}, CONCURRENCY)
	subRdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	sub = subRdb.Subscribe(ctx, "v2xfer")
	ch1 := sub.Channel()

	// idFunds = ordinals.RefreshV2Funding(CONCURRENCY)

	fundsJson := rdb.HGetAll(ctx, "v2funds").Val()
	for id, j := range fundsJson {
		funds := ordinals.V2TokenFunds{}
		err := json.Unmarshal([]byte(j), &funds)
		if err != nil {
			log.Panic(err)
		}
		pkhash := hex.EncodeToString(funds.PKHash)
		pkhashFunds[pkhash] = &funds
		m.Lock()
		idFunds[id] = &funds
		m.Unlock()
		sub.Subscribe(ctx, pkhash)
	}

	go func() {
		for {
			m.Lock()
			idFunds = ordinals.InitializeV2Funding(CONCURRENCY)
			for _, funds := range idFunds {
				pkhash := hex.EncodeToString(funds.PKHash)
				if _, ok := pkhashFunds[pkhash]; !ok {
					pkhashFunds[pkhash] = funds
					sub.Subscribe(ctx, pkhash)
				}
			}
			m.Unlock()
			time.Sleep(time.Hour)
		}
	}()

	go func() {
		for msg := range ch1 {
			switch msg.Channel {
			case "v2funds":
				funds := &ordinals.V2TokenFunds{}
				err := json.Unmarshal([]byte(msg.Payload), &funds)
				if err != nil {
					break
				}
				m.Lock()
				idFunds[funds.Id.String()] = funds
				m.Unlock()
				pkhash := hex.EncodeToString(funds.PKHash)
				pkhashFunds[pkhash] = funds
			case "v2xfer":
				parts := strings.Split(msg.Payload, ":")
				txid, err := hex.DecodeString(parts[0])
				if err != nil {
					log.Println("Decode err", err)
					break
				}
				tokenId, err := lib.NewOutpointFromString(parts[1])
				if err != nil {
					log.Println("NewOutpointFromString err", err)
					break
				}
				if funds, ok := idFunds[tokenId.String()]; ok {
					outputs := ordinals.ValidateV2Transfer(txid, tokenId, false)
					funds.Used += int64(outputs) * ordinals.BSV20V2_OP_COST
				}
			default:
				if funds, ok := pkhashFunds[msg.Channel]; ok {
					log.Println("Updating funding", funds.Id.String())
					funds.UpdateFunding()
				}
			}
		}
	}()

	go func() {
		for {
			if !processV2() {
				log.Println("No work to do")
				time.Sleep(time.Minute)
			}
		}
	}()

	err := indexer.Exec(
		true,
		true,
		func(ctx *lib.IndexContext) error {
			ordinals.IndexInscriptions(ctx)
			ordinals.IndexBsv20(ctx)
			return nil
		},
		func(height uint32) error {
			currentHeight = height
			return nil
		},
		INDEXER,
		TOPIC,
		FROM_BLOCK,
		CONCURRENCY,
		false,
		false,
		VERBOSE)
	if err != nil {
		log.Panicln(err)
	}
}

func processV2() (didWork bool) {
	var wg sync.WaitGroup
	m.Lock()
	var fundsList = make([]*ordinals.V2TokenFunds, 0, len(idFunds))
	for _, funds := range idFunds {
		if funds.Balance() < ordinals.BSV20V2_OP_COST {
			continue
		}
		fundsList = append(fundsList, funds)
	}
	m.Unlock()

	for _, funds := range fundsList {
		if funds.Balance() < ordinals.BSV20V2_OP_COST {
			continue
		}

		log.Println("Processing V2", funds.Id.String(), funds.Balance())
		wg.Add(1)
		limiter <- struct{}{}
		go func(funds *ordinals.V2TokenFunds) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			rows, err := db.Query(ctx, `
				SELECT txid, vout, height, idx, id, amt
				FROM bsv20_txos
				WHERE op='transfer' AND id=$1 AND status=0
				ORDER BY height ASC, idx ASC, vout ASC
				LIMIT $2`,
				funds.Id,
				funds.Balance()/ordinals.BSV20V2_OP_COST,
			)
			if err != nil {
				log.Panic(err)
			}
			defer rows.Close()

			var prevTxid []byte
			for rows.Next() {
				bsv20 := &ordinals.Bsv20{}
				err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Id, &bsv20.Amt)
				if err != nil {
					log.Panicln(err)
				}
				// fmt.Printf("Validating Transfer: %s %x\n", funds.Id.String(), bsv20.Txid)

				if bytes.Equal(prevTxid, bsv20.Txid) {
					// fmt.Printf("Skipping: %s %x\n", funds.Id.String(), bsv20.Txid)
					continue
				}
				prevTxid = bsv20.Txid
				outputs := ordinals.ValidateV2Transfer(bsv20.Txid, funds.Id, bsv20.Height != nil && *bsv20.Height <= currentHeight)
				if outputs > 0 {
					didWork = true
				}
				funds.Used += int64(outputs) * ordinals.BSV20V2_OP_COST
				fmt.Printf("Validated Transfer: %s %x\n", funds.Id.String(), bsv20.Txid)
			}

			if didWork {
				funds.UpdateFunding()
			}
		}(funds)
	}
	wg.Wait()

	return
}
