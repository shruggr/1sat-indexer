package main

import (
	"bytes"
	"context"
	"encoding/hex"
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
var CONCURRENCY int = 8
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	// flag.IntVar(&CONCURRENCY, "c", 32, "Concurrency Limit")
	// flag.IntVar(&VERBOSE, "v", 0, "Verbose")
	// flag.Parse()

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
		Addr:     "localhost:6379",
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

var pkhashFunds = map[string]*ordinals.V1TokenFunds{}
var tickFunds map[string]*ordinals.V1TokenFunds

var limiter = make(chan struct{}, CONCURRENCY)
var sub *redis.PubSub

func main() {
	tickFunds = ordinals.InitializeV1Funding(CONCURRENCY)

	go func() {
		for {
			time.Sleep(time.Hour)
			tickFunds = ordinals.InitializeV1Funding(CONCURRENCY)
		}
	}()

	subRdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	sub = subRdb.Subscribe(context.Background(), "v1xfer")
	ch1 := sub.Channel()
	for _, funds := range tickFunds {
		pkhash := hex.EncodeToString(funds.PKHash)
		pkhashFunds[pkhash] = funds
		sub.Subscribe(ctx, pkhash)
	}
	go func() {
		for msg := range ch1 {
			if msg.Channel == "v1xfer" {
				parts := strings.Split(msg.Payload, ":")
				txid, err := hex.DecodeString(parts[0])
				if err != nil {
					continue
				}
				tick := parts[1]
				if funds, ok := tickFunds[tick]; ok {
					outputs := ordinals.ValidateV1Transfer(txid, tick, false)
					funds.Used += int64(outputs) * ordinals.BSV20V1_OP_COST
				}
				continue
			}
			if funds, ok := pkhashFunds[msg.Channel]; ok {
				funds.UpdateFunding()
			}
		}
	}()

	for {
		if !processV1() {
			log.Println("No work to do")
			time.Sleep(time.Minute)
		}
	}
}

func processV1() (didWork bool) {
	var wg sync.WaitGroup
	for _, funds := range tickFunds {
		if funds.Balance() < ordinals.BSV20V1_OP_COST {
			continue
		}

		log.Println("Processing V1", funds.Tick, funds.Balance())
		wg.Add(1)
		limiter <- struct{}{}
		go func(funds *ordinals.V1TokenFunds) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			row := db.QueryRow(ctx, `
				SELECT COUNT(1)
				FROM bsv20_txos
				WHERE tick=$1 AND status=0 AND height > 0 AND height IS NOT NULL`,
				funds.Tick,
			)
			var pending int
			err := row.Scan(&pending)
			if err != nil {
				log.Panic(err)
			}

			limit := (funds.Balance() - int64(pending)*ordinals.BSV20V1_OP_COST) / ordinals.BSV20V1_OP_COST

			if limit > 0 {
				rows, err := db.Query(ctx, `
					SELECT txid, height, idx, txouts
					FROM bsv20v1_txns
					WHERE processed=false AND tick=$1
					ORDER BY height ASC, idx ASC
					LIMIT $2`,
					funds.Tick,
					limit,
				)
				if err != nil {
					log.Panic(err)
				}
				defer rows.Close()
				balance := funds.Balance()

				for rows.Next() {
					var txid []byte
					var height uint32
					var idx uint64
					var txouts int64
					err = rows.Scan(&txid, &height, &idx, &txouts)
					if err != nil {
						log.Panic(err)
					}
					if balance < txouts*ordinals.BSV20V1_OP_COST {
						break
					}
					log.Printf("Processing %s %x\n", funds.Tick, txid)
					rawtx, err := lib.LoadRawtx(hex.EncodeToString(txid))
					if err != nil {
						log.Panic(err)
					}

					txn := ordinals.IndexTxn(rawtx, "", height, idx)
					ordinals.ParseBsv20(txn)
					for _, txo := range txn.Txos {
						if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
							if bsv20.Ticker != funds.Tick {
								continue
							}
							if bsv20.Op == "transfer" || bsv20.Op == "mint" {
								balance -= ordinals.BSV20V1_OP_COST
							}
							bsv20.Save(txo)
						}
					}
					_, err = db.Exec(ctx, `
					UPDATE bsv20v1_txns
					SET processed=true
					WHERE txid=$1 AND tick=$2`,
						txn.Txid,
						funds.Tick,
					)
					if err != nil {
						log.Panic(err)
					}
					didWork = true
					if balance < ordinals.BSV20V1_OP_COST {
						break
					}
				}
				rows.Close()
			}
			ticker := ordinals.LoadTicker(funds.Tick)
			rows, err := db.Query(ctx, `
				SELECT txid, vout, height, idx, tick, amt, op
				FROM bsv20_txos
				WHERE tick=$1 AND status=0 AND height > 0 AND height IS NOT NULL
				ORDER BY height ASC, idx ASC, vout ASC
				LIMIT $2`,
				funds.Tick,
				funds.Balance()/ordinals.BSV20V1_OP_COST,
			)
			if err != nil {
				log.Panic(err)
			}
			defer rows.Close()

			var prevTxid []byte
			for rows.Next() {
				bsv20 := &ordinals.Bsv20{}
				err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Ticker, &bsv20.Amt, &bsv20.Op)
				// err = rows.Scan(&txid, &vout, &op)
				if err != nil {
					log.Panicln(err)
				}

				// log.Println("Validating", funds.Tick, bsv20.Vout, bsv20.Op)
				var reason string
				switch bsv20.Op {
				case "mint":
					if ticker == nil {
						reason = fmt.Sprintf("invalid ticker %s as of %d %d", funds.Tick, &bsv20.Height, &bsv20.Idx)
					} else if ticker.Supply >= ticker.Max {
						reason = fmt.Sprintf("supply %d >= max %d", ticker.Supply, ticker.Max)
					} else if ticker.Limit > 0 && *bsv20.Amt > ticker.Limit {
						reason = fmt.Sprintf("amt %d > limit %d", *bsv20.Amt, ticker.Limit)
					}

					if reason != "" {
						_, err = db.Exec(ctx, `
							UPDATE bsv20_txos
							SET status=-1, reason=$3
							WHERE txid=$1 AND vout=$2`,
							bsv20.Txid,
							bsv20.Vout,
							reason,
						)
						if err != nil {
							log.Panic(err)
						}
						continue
					}

					t, err := db.Begin(ctx)
					if err != nil {
						log.Panic(err)
					}
					defer t.Rollback(ctx)

					if ticker.Max-ticker.Supply < *bsv20.Amt {
						reason = fmt.Sprintf("supply %d + amt %d > max %d", ticker.Supply, *bsv20.Amt, ticker.Max)
						*bsv20.Amt = ticker.Max - ticker.Supply

						_, err := t.Exec(ctx, `
							UPDATE bsv20_txos
							SET status=1, amt=$3, reason=$4
							WHERE txid=$1 AND vout=$2`,
							bsv20.Txid,
							bsv20.Vout,
							*bsv20.Amt,
							reason,
						)
						if err != nil {
							log.Panic(err)
						}
					} else {
						_, err := t.Exec(ctx, `
							UPDATE bsv20_txos
							SET status=1
							WHERE txid=$1 AND vout=$2`,
							bsv20.Txid,
							bsv20.Vout,
						)
						if err != nil {
							log.Panic(err)
						}
					}

					ticker.Supply += *bsv20.Amt
					_, err = t.Exec(ctx, `
							UPDATE bsv20
							SET supply=$3
							WHERE txid=$1 AND vout=$2`,
						ticker.Txid,
						ticker.Vout,
						ticker.Supply,
					)
					if err != nil {
						log.Panic(err)
					}

					err = t.Commit(ctx)
					if err != nil {
						log.Panic(err)
					}
					funds.Used += ordinals.BSV20V1_OP_COST
					rdb.IncrBy(ctx, "funds:"+funds.Tick, int64(ordinals.BSV20V1_OP_COST*-1))
					fmt.Println("Validated Mint:", funds.Tick, ticker.Supply, ticker.Max)
					didWork = true
				case "transfer":
					if bytes.Equal(prevTxid, bsv20.Txid) {
						continue
					}
					prevTxid = bsv20.Txid
					outputs := ordinals.ValidateV1Transfer(bsv20.Txid, funds.Tick, true)
					funds.Used += int64(outputs) * ordinals.BSV20V1_OP_COST
					fmt.Printf("Validated Transfer: %s %x\n", funds.Tick, bsv20.Txid)
					didWork = true
				}
			}
		}(funds)
	}
	wg.Wait()

	return
}
