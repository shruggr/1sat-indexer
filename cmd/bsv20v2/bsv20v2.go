package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
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

var limiter chan struct{}
var sub *redis.PubSub

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
		idFunds[id] = &funds
		sub.Subscribe(ctx, pkhash)
	}

	go func() {
		for {
			idFunds = ordinals.InitializeV2Funding(CONCURRENCY)
			for _, funds := range idFunds {
				pkhash := hex.EncodeToString(funds.PKHash)
				if _, ok := pkhashFunds[pkhash]; !ok {
					pkhashFunds[pkhash] = funds
					sub.Subscribe(ctx, pkhash)
				}
			}
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
				idFunds[funds.Id.String()] = funds
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

	for {
		if !processV2() {
			log.Println("No work to do")
			time.Sleep(time.Minute)
		}
	}
}

func processV2() (didWork bool) {
	var wg sync.WaitGroup
	for _, funds := range idFunds {
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
			row := db.QueryRow(ctx, `
				SELECT COUNT(1)
				FROM bsv20_txos
				WHERE id=$1 AND status=0 AND height>0 AND height IS NOT NULL`,
				funds.Id,
			)
			var pending int
			err := row.Scan(&pending)
			if err != nil {
				log.Panic(err)
			}

			limit := (funds.Balance() - int64(pending)*ordinals.BSV20V2_OP_COST) / ordinals.BSV20V2_OP_COST
			if limit > 0 {
				rows, err := db.Query(ctx, `
					SELECT txid, height, idx, txouts
					FROM bsv20v2_txns
					WHERE processed=false AND id=$1
					ORDER BY height ASC, idx ASC
					LIMIT $2`,
					funds.Id,
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
					if balance < txouts*ordinals.BSV20V2_OP_COST {
						// log.Printf("Insufficient Balance %s %x\n", funds.Id.String(), txid)
						break
					}
					// log.Printf("Loading %s %x\n", funds.Id.String(), txid)
					rawtx, err := lib.LoadRawtx(hex.EncodeToString(txid))
					if err != nil {
						log.Panic(err)
					}

					// log.Printf("Parsing %s %x\n", funds.Id.String(), txid)
					txn := ordinals.IndexTxn(rawtx, "", height, idx)
					ordinals.ParseBsv20(txn)
					// log.Printf("Parsed %s %x\n", funds.Id.String(), txid)
					for _, txo := range txn.Txos {
						if bsv20, ok := txo.Data["bsv20"].(*ordinals.Bsv20); ok {
							if !bytes.Equal(*funds.Id, *bsv20.Id) {
								continue
							}
							if bsv20.Op == "transfer" {
								balance -= ordinals.BSV20V2_OP_COST
							}
							// log.Printf("Saving %s %x %d\n", funds.Id.String(), txid, txo.Outpoint.Vout())
							bsv20.Save(txo)
						}
					}
					// log.Printf("Updating %s %x\n", funds.Id.String(), txid)
					_, err = db.Exec(ctx, `
					UPDATE bsv20v2_txns
					SET processed=true
					WHERE txid=$1 AND id=$2`,
						txn.Txid,
						funds.Id,
					)
					if err != nil {
						log.Panic(err)
					}
					didWork = true
					if balance < ordinals.BSV20V2_OP_COST {
						break
					}
				}
				rows.Close()
			}

			rows, err := db.Query(ctx, `
				SELECT txid, vout, height, idx, id, amt
				FROM bsv20_txos
				WHERE op='transfer' AND id=$1 AND status=0 AND height > 0 AND height IS NOT NULL
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
			hasRows := false
			for rows.Next() {
				hasRows = true
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
				didWork = true
				prevTxid = bsv20.Txid
				outputs := ordinals.ValidateV2Transfer(bsv20.Txid, funds.Id, true)
				funds.Used += int64(outputs) * ordinals.BSV20V2_OP_COST
				fmt.Printf("Validated Transfer: %s %x\n", funds.Id.String(), bsv20.Txid)
			}

			if hasRows {
				funds.UpdateFunding()
			}
		}(funds)
	}
	wg.Wait()

	return
}
