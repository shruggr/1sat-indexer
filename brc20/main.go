package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/GorillaPool/go-junglebus"
	jbModels "github.com/GorillaPool/go-junglebus/models"
	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

const INDEXER = "bsv20"

// const TRIGGER = 792700

var THREADS uint64 = 16

var db *sql.DB
var junglebusClient *junglebus.Client
var msgQueue = make(chan *Msg, 1000000)

// var settled = make(chan uint32, 100)
var fromBlock uint32
var sub *junglebus.Subscription

type Msg struct {
	Id          string
	Height      uint32
	Hash        string
	Status      uint32
	Idx         uint32
	Transaction []byte
}

// var ctx = context.Background()
var rdb *redis.Client

func init() {
	godotenv.Load("../.env")

	var err error
	db, err = sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(db, rdb)
	if err != nil {
		log.Panic(err)
	}

	if os.Getenv("THREADS") != "" {
		THREADS, err = strconv.ParseUint(os.Getenv("THREADS"), 10, 64)
		if err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	var err error
	junglebusClient, err = junglebus.New(
		junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
	)
	if err != nil {
		log.Panicln(err.Error())
	}
	row := db.QueryRow(`SELECT height
		FROM progress
		WHERE indexer=$1`,
		INDEXER,
	)
	row.Scan(&fromBlock)
	// fromBlock = lib.TRIGGER
	// fromBlock = 792000

	fmt.Println("Starting from block", fromBlock)
	go processQueue()
	// fmt.Println("Subscribing...")
	subscribe()
	// fmt.Printf("Subscribed to %s\n", os.Getenv("BSV20"))

	defer func() {
		if r := recover(); r != nil {
			sub.Unsubscribe()
			fmt.Println("Recovered in f", r)
			fmt.Println("Unsubscribing and exiting...")
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Printf("Caught signal")
		fmt.Println("Unsubscribing and exiting...")
		sub.Unsubscribe()
		os.Exit(0)
	}()

	var wg2 sync.WaitGroup
	wg2.Add(1)
	wg2.Wait()
}

func subscribe() {
	var err error
	log.Println("Subscribing to Junglebus from block", fromBlock)
	sub, err = junglebusClient.Subscribe(
		context.Background(),
		os.Getenv("BSV20"),
		uint64(fromBlock),
		junglebus.EventHandler{
			OnTransaction: func(txResp *jbModels.TransactionResponse) {
				// log.Printf("[TX]: %d - %d: %s\n", txResp.BlockHeight, txResp.BlockIndex, txResp.Id)
				msgQueue <- &Msg{
					Id:          txResp.Id,
					Height:      txResp.BlockHeight,
					Idx:         uint32(txResp.BlockIndex),
					Transaction: txResp.Transaction,
				}
			},
			OnStatus: func(status *jbModels.ControlResponse) {
				// log.Printf("[STATUS]: %v\n", status)
				if status.StatusCode == 999 {
					log.Println(status.Message)
					log.Println("Unsubscribing...")
					sub.Unsubscribe()
					os.Exit(0)
					return
				}
				msgQueue <- &Msg{
					Height: status.Block,
					Status: status.StatusCode,
				}
			},
			OnError: func(err error) {
				log.Printf("[ERROR]: %v", err)
			},
		},
	)
	if err != nil {
		log.Panic(err)
	}
}

func processQueue() {
	// txid, _ := hex.DecodeString("c67cae20d5f6be98342abb6e1339b016a287f9e528317aa3214a1b8cbfc9100a")
	// txData, err := lib.LoadTxData(txid)
	// if err != nil {
	// 	log.Panic(err)
	// }
	// tx, err := bt.NewTxFromBytes(txData.Transaction)
	// if err != nil {
	// 	log.Panic(err)
	// }

	var settledHeight uint32
	getBsv20, err := db.Prepare(`SELECT
		FROM bsv20
		WHERE tick=UPPER($1)`)
	if err != nil {
		log.Panic(err)
	}

	setSpend, err := db.Prepare(`UPDATE bsv20_txos
		SET spend=$3
		WHERE txid=$1 AND vout=$2
		RETURNING amt, tick`)
	if err != nil {
		log.Panic(err)
	}

	insBsv20, err := db.Prepare(`INSERT INTO bsv20(id, tick, max, lim, dec, height, idx)
		VALUES($1, UPPER($2), $3, $4, $5, $6, $7)
		ON CONFLICT(id) DO NOTHING`)
	if err != nil {
		log.Panic(err)
	}

	insBsv20Txo, err := db.Prepare(`INSERT INTO bsv20_txos(txid, vout, op, tick, amt, lock)
		VALUES($1, $2, $3, UPPER($4), $5, $6)
		ON CONFLICT(txid, vout) DO NOTHING`)
	if err != nil {
		log.Panic(err)
	}

	getBsv20Supply, err := db.Prepare(`SELECT COALESCE(SUM(amt), 0)
		FROM bsv20_txos
		WHERE tick=UPPER($1) AND op='mint'`)
	if err != nil {
		log.Panic(err)
	}

	// go indexer.ProcessInscriptionIds(settled)
	// go indexer.ProcessTxns(uint(THREADS))
msgLoop:
	for {
		msg := <-msgQueue
		// fmt.Println("MSG", msg.Id, msg.Status)
		switch msg.Status {
		case 0:
			// log.Printf("[TX]: %d - %d: %s\n", msg.Height, msg.Idx, msg.Id)
			tx, err := bt.NewTxFromBytes(msg.Transaction)
			if err != nil {
				log.Panicf("OnTransaction Parse Error: %s %d %+v\n", msg.Id, len(msg.Transaction), err)
			}
			txid := tx.TxIDBytes()
			tokenBalance := map[string]uint64{}
			var m sync.Mutex
			for _, input := range tx.Inputs {
				rows, err := setSpend.Query(input.PreviousTxID(), input.PreviousTxOutIndex, txid)
				if err != nil {
					log.Panic(err)
				}
				if rows.Next() {
					in := &Bsv20{}
					err = rows.Scan(&in.Amt, &in.Ticker)
					if err != nil {
						log.Panic(err)
					}
					m.Lock()
					if amt, ok := tokenBalance[in.Ticker]; ok {
						tokenBalance[in.Ticker] = amt + in.Amt
					} else {
						tokenBalance[in.Ticker] = in.Amt
					}
					m.Unlock()
				}
				rows.Close()
			}

			tokenSupply := map[string]uint64{}
			bsv20Out := []*Bsv20{}
			for vout, output := range tx.Outputs {
				bsv20, err := parseBsv20(*output.LockingScript, tx, uint32(vout))
				if err != nil {
					log.Panic(err)
				}
				if bsv20 == nil {
					continue
				}

				switch bsv20.Op {
				case "deploy":
					rows, err := getBsv20.Query(bsv20.Ticker)
					if err != nil {
						log.Panic(err)
					}
					if rows.Next() {
						rows.Close()
						continue
					}
					rows.Close()

					bsv20.Id = lib.NewOutpoint(txid, uint32(vout))
					_, err = insBsv20.Exec(bsv20.Id, bsv20.Ticker, bsv20.Max, bsv20.Limit, bsv20.Decimals, msg.Height, msg.Idx)
					if err != nil {
						log.Panic(err)
					}
				case "mint":
					var supply uint64
					if amt, ok := tokenSupply[bsv20.Ticker]; ok {
						supply = amt
					} else {
						row := getBsv20Supply.QueryRow(bsv20.Ticker)
						err = row.Scan(&supply)
						if err != nil {
							log.Panic(err)
						}
					}
					if supply > bsv20.Max {
						continue
					} else if supply+bsv20.Amt > bsv20.Max {
						bsv20.Amt = bsv20.Max - supply
					}
					supply += bsv20.Amt
					tokenSupply[bsv20.Ticker] = supply
					bsv20Out = append(bsv20Out, bsv20)
				case "transfer":
					if balance, ok := tokenBalance[bsv20.Ticker]; ok && balance >= bsv20.Amt {
						balance -= bsv20.Amt
						tokenBalance[bsv20.Ticker] = balance
						bsv20Out = append(bsv20Out, bsv20)
					} else {
						continue msgLoop
						// continue
					}
				}
			}
			for _, bsv20 := range bsv20Out {
				fmt.Println("BSV20:", bsv20.Ticker, bsv20.Amt)
				_, err = insBsv20Txo.Exec(bsv20.Txid, bsv20.Vout, bsv20.Ticker, bsv20.Op, bsv20.Amt, bsv20.Lock)
				if err != nil {
					log.Panic(err)
				}
				// out, err := json.MarshalIndent(bsv20, "", "  ")
				// if err != nil {
				// 	log.Panic(err)
				// }
				// log.Panicln(string(out))
			}

		case 200:
			indexer.Wg.Wait()
			settledHeight = msg.Height - 6

			if _, err := db.Exec(`INSERT INTO progress(indexer, height)
				VALUES($1, $2)
				ON CONFLICT(indexer) DO UPDATE
					SET height=$2`,
				INDEXER,
				settledHeight,
			); err != nil {
				log.Panic(err)
			}
			fromBlock = msg.Height + 1
			fmt.Printf("Completed: %d\n", msg.Height)
			// settled <- settledHeight

		default:
			log.Printf("Status: %d\n", msg.Status)
		}
	}
}
