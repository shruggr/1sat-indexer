package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

// var settled = make(chan uint32, 1000)
var POSTGRES string
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
	db, err := pgxpool.New(ctx, POSTGRES)
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
	if err != nil {
		log.Panic(err)
	}
}

var pkhashFunds = map[string]*ordinals.V1TokenFunds{}
var tickFunds = map[string]*ordinals.V1TokenFunds{}

var limiter = make(chan struct{}, CONCURRENCY)
var sub *redis.PubSub
var m sync.Mutex

func main() {
	subRdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	sub = subRdb.Subscribe(context.Background(), "v1xfer", "v1funds")
	ch1 := sub.Channel()

	fundsJson := lib.Rdb.HGetAll(ctx, "v1funds").Val()
	for tick, j := range fundsJson {
		funds := ordinals.V1TokenFunds{}
		err := json.Unmarshal([]byte(j), &funds)
		if err != nil {
			log.Panic(err)
		}
		pkhash := hex.EncodeToString(funds.PKHash)
		pkhashFunds[pkhash] = &funds
		m.Lock()
		tickFunds[tick] = &funds
		m.Unlock()
		sub.Subscribe(ctx, pkhash)
	}

	go func() {
		for {
			tickFunds = ordinals.InitializeV1Funding(CONCURRENCY)
			m.Lock()
			for _, funds := range tickFunds {
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

	throttle := map[string]time.Time{}

	go func() {
		for msg := range ch1 {
			switch msg.Channel {
			case "v1funds":
				funds := &ordinals.V1TokenFunds{}
				err := json.Unmarshal([]byte(msg.Payload), &funds)
				if err != nil {
					continue
				}
				// log.Println("Updating funding", funds.Tick, funds.Balance())
				m.Lock()
				tickFunds[funds.Tick] = funds
				m.Unlock()
				pkhash := hex.EncodeToString(funds.PKHash)
				pkhashFunds[pkhash] = funds
			case "v1xfer":
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
			default:
				if funds, ok := pkhashFunds[msg.Channel]; ok {
					if t, ok := throttle[msg.Channel]; !ok || time.Since(t) > 10*time.Second {
						throttle[msg.Channel] = time.Now()
						log.Println("Updating funding", funds.Tick)
						funds.UpdateFunding()
					}
				}
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

	row := lib.Db.QueryRow(ctx, `
		SELECT height
		FROM progress
		WHERE indexer=$1`,
		"bsv20",
	)
	var crawledHeight uint32
	if err := row.Scan(&crawledHeight); err != nil {
		log.Panic(err)
	}

	m.Lock()
	var fundsList = make([]*ordinals.V1TokenFunds, 0, len(tickFunds))
	for _, funds := range tickFunds {
		if funds.Balance() < ordinals.BSV20V1_OP_COST {
			continue
		}
		fundsList = append(fundsList, funds)
	}
	m.Unlock()

	for _, funds := range fundsList {
		log.Println("Processing V1", funds.Tick, funds.Balance())
		wg.Add(1)
		limiter <- struct{}{}
		go func(funds *ordinals.V1TokenFunds) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			row := lib.Db.QueryRow(ctx, `
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
			// log.Print("Pending", funds.Tick, pending)

			limit := (funds.Balance() - int64(pending)*ordinals.BSV20V1_OP_COST) / ordinals.BSV20V1_OP_COST

			// log.Println("Balance V1", funds.Tick, limit, funds.Balance())
			if limit > 0 {
				rows, err := lib.Db.Query(ctx, `
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
					log.Printf("Processing %s %x %d %d\n", funds.Tick, txid, balance, txouts)
					if balance < txouts*ordinals.BSV20V1_OP_COST {
						log.Println("Insufficient balance", funds.Tick, balance, txouts*ordinals.BSV20V1_OP_COST)
						break
					}
					rawtx, err := lib.LoadRawtx(hex.EncodeToString(txid))
					if err != nil {
						log.Panic(err)
					}

					txn := ordinals.IndexTxn(rawtx, "", height, idx)
					ordinals.IndexBsv20(txn)
					for _, txo := range txn.Txos {
						if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
							if bsv20.Ticker != funds.Tick {
								continue
							}
							if slices.Contains([]string{"transfer", "mint", "burn"}, bsv20.Op) {
								balance -= ordinals.BSV20V1_OP_COST
							}
						}
					}
					_, err = lib.Db.Exec(ctx, `
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
			rows, err := lib.Db.Query(ctx, `
				SELECT txid, vout, height, idx, tick, amt, op
				FROM bsv20_txos
				WHERE tick=$1 AND op='mint' AND status=0 AND height > 0 AND height IS NOT NULL
				ORDER BY height ASC, idx ASC, vout ASC
				LIMIT $2`,
				funds.Tick,
				funds.Balance()/ordinals.BSV20V1_OP_COST,
			)
			if err != nil {
				log.Panic(err)
			}
			defer rows.Close()

			for rows.Next() {
				bsv20 := &ordinals.Bsv20{}
				err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Ticker, &bsv20.Amt, &bsv20.Op)
				if err != nil {
					log.Panicln(err)
				}

				didWork = true
				var reason string
				if ticker == nil {
					reason = fmt.Sprintf("invalid ticker %s as of %d %d", funds.Tick, &bsv20.Height, &bsv20.Idx)
				} else if ticker.Supply >= ticker.Max {
					reason = fmt.Sprintf("supply %d >= max %d", ticker.Supply, ticker.Max)
				} else if ticker.Limit > 0 && *bsv20.Amt > ticker.Limit {
					reason = fmt.Sprintf("amt %d > limit %d", *bsv20.Amt, ticker.Limit)
				}

				if reason != "" {
					_, err = lib.Db.Exec(ctx, `
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

				t, err := lib.Db.Begin(ctx)
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
						log.Panicf("%x %d %v\n", bsv20.Txid, bsv20.Vout, err)
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
				fmt.Println("Validated Mint:", funds.Tick, ticker.Supply, ticker.Max)
				didWork = true

			}
			rows.Close()

			rows, err = lib.Db.Query(ctx, `
				SELECT txid, vout, height, idx, tick, amt, op
				FROM bsv20_txos
				WHERE tick=$1 AND op='transfer' AND status=0
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
				if err != nil {
					log.Panicln(err)
				}

				if bytes.Equal(prevTxid, bsv20.Txid) {
					continue
				}
				prevTxid = bsv20.Txid
				outputs := ordinals.ValidateV1Transfer(bsv20.Txid, funds.Tick, bsv20.Height != nil && *bsv20.Height <= crawledHeight)
				funds.Used += int64(outputs) * ordinals.BSV20V1_OP_COST
				fmt.Printf("Validated Transfer: %s %x\n", funds.Tick, bsv20.Txid)
				if outputs > 0 {
					didWork = true
				}
			}
			if didWork {
				funds.UpdateFunding()
			}
		}(funds)
	}
	wg.Wait()

	return
}
