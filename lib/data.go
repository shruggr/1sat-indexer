package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
)

var TRIGGER = uint32(783968)
var JUNGLEBUS string

// var Db *pgxpool.Pool
var Rdb *redis.Client
var Cache *redis.Client
var Queue *redis.Client
var JB *junglebus.Client
var bit *bitcoin.Bitcoind

func Initialize() (err error) {
	// Db = postgres
	// Rdb = rdb
	// Cache = cache
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
	log.Println("JUNGLEBUS", JUNGLEBUS)
	if JUNGLEBUS != "" {
		JB, err = junglebus.New(
			junglebus.WithHTTP(JUNGLEBUS),
		)
		if err != nil {
			return
		}
	}

	log.Println("REDISDB", os.Getenv("REDISDB"))
	Rdb = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	log.Println("REDISQUEUE", os.Getenv("REDISQUEUE"))
	Queue = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISQUEUE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	log.Println("REDISCACHE", os.Getenv("REDISCACHE"))
	Cache = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISCACHE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	if os.Getenv("BITCOIN_HOST") != "" {
		port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
		bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
		if err != nil {
			log.Panic(err)
		}
	}

	return
}

func LoadTx(ctx context.Context, txid string, withProof bool) (tx *transaction.Transaction, err error) {
	var rawtx []byte
	if rawtx, err = LoadRawtx(ctx, txid); err != nil {
		return
	} else if len(rawtx) == 0 {
		fmt.Printf("missing-txn %s\n", txid)
		return
	} else if tx, err = transaction.NewTransactionFromBytes(rawtx); err != nil {
		return
	}
	if withProof {
		if proof, err := LoadProof(ctx, txid); err == nil {
			tx.MerklePath = proof
		}
	}

	return
}

func LoadRawtx(ctx context.Context, txid string) (rawtx []byte, err error) {
	cacheKey := TxKey(txid)
	rawtx, _ = Cache.Get(ctx, cacheKey).Bytes()

	if len(rawtx) > 100 {
		return rawtx, nil
	} else {
		rawtx = []byte{}

	}

	if len(rawtx) == 0 && JB != nil {
		url := fmt.Sprintf("%s/v1/transaction/get/%s/bin", os.Getenv("JUNGLEBUS"), txid)
		start := time.Now()
		if resp, err := http.Get(url); err == nil && resp.StatusCode < 300 {
			rawtx, _ = io.ReadAll(resp.Body)
		}
		log.Println("Requesting rawtx", txid, time.Since(start))
	}

	if len(rawtx) == 0 && bit != nil {
		// log.Println("Requesting tx from node", txid)
		if r, err := bit.GetRawTransactionRest(txid); err == nil {
			rawtx, _ = io.ReadAll(r)
		}
	}

	if len(rawtx) > 0 {
		Cache.Set(ctx, cacheKey, rawtx, 0).Err()
	}

	return
}

func LoadProof(ctx context.Context, txid string) (proof *transaction.MerklePath, err error) {
	cacheKey := ProofKey(txid)
	prf, _ := Cache.Get(ctx, cacheKey).Bytes()
	if len(prf) == 0 && JB != nil {
		start := time.Now()
		url := fmt.Sprintf("%s/v1/transaction/proof/%s/bin", os.Getenv("JUNGLEBUS"), txid)
		if resp, err := http.Get(url); err == nil && resp.StatusCode < 300 {
			prf, _ = io.ReadAll(resp.Body)
		}
		log.Println("Requesting proof", txid, time.Since(start))
	}
	if len(prf) > 0 {
		if proof, err = transaction.NewMerklePathFromBinary(prf); err != nil {
			return
		} else if chaintip, err := Rdb.ZRangeWithScores(ctx, BlockHeightKey, -1, -1).Result(); err != nil {
			return nil, err
		} else if proof.BlockHeight < uint32(chaintip[0].Score-5) {
			Cache.Set(ctx, cacheKey, prf, 0)
		} else {
			Cache.Set(ctx, cacheKey, prf, time.Hour)
		}

	}
	return
}

func LoadTxOut(outpoint *Outpoint) (txout *transaction.TransactionOutput, err error) {
	url := fmt.Sprintf("https://junglebus.gorillapool.io/v1/txo/get/%s", outpoint.String())
	// log.Println("Requesting txo", url)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("missing-txn %s", outpoint.String())
		return
	}
	txout = &transaction.TransactionOutput{}
	_, err = txout.ReadFrom(resp.Body)
	return
}

// func GetSpend(outpoint *Outpoint) (spend []byte, err error) {
// 	resp, err := http.Get(fmt.Sprintf("%s/v1/txo/spend/%s", os.Getenv("JUNGLEBUS"), outpoint.String()))
// 	if err != nil {
// 		log.Println("JB Spend Request", err)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode >= 300 {
// 		err = fmt.Errorf("missing-spend-%s", outpoint.String())
// 		return
// 	}
// 	return io.ReadAll(resp.Body)
// }

func FetchBlockHeaders(fromBlock uint64, pageSize uint) (blocks []*BlockHeader, err error) {
	url := fmt.Sprintf("%s/v1/block_header/list/%d?limit=%d", JUNGLEBUS, fromBlock, pageSize)
	log.Printf("Requesting %d blocks from height %d\n", pageSize, fromBlock)
	if resp, err := http.Get(url); err != nil || resp.StatusCode != 200 {
		log.Panicln("Failed to get blocks from junglebus", resp.StatusCode, err)
	} else {
		err := json.NewDecoder(resp.Body).Decode(&blocks)
		resp.Body.Close()
		if err != nil {
			log.Panic(err)
		}
	}
	return
}

func GetChaintip(ctx context.Context) *models.BlockHeader {
	chaintip := &models.BlockHeader{}
	if data, err := Rdb.Get(ctx, "chaintip").Bytes(); err != nil {
		log.Panic(err)
	} else if err = json.Unmarshal(data, &chaintip); err != nil {
		log.Panic(err)
	}
	return chaintip
}

func FetchOwnerTxns(address string, lastHeight int) (txns []*AddressTxn, err error) {
	url := fmt.Sprintf("%s/v1/address/get/%s/%d", JUNGLEBUS, address, lastHeight)
	if resp, err := http.Get(url); err != nil {
		log.Panic(err)
	} else if resp.StatusCode != 200 {
		log.Panic("Bad status", resp.StatusCode)
	} else {
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&txns); err != nil {
			log.Panic(err)
		}
	}
	return
}

// func PublishEvent(ctx context.Context, event string, data string) {
// 	eventId := time.Now().Unix()
// 	cutoff := time.Now().Add(-24 * time.Hour)
// 	eventKey := "evt:" + event
// 	Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
// 		pipe.ZRemRangeByScore(ctx, eventKey, "-inf", fmt.Sprintf("%d", cutoff.Unix()))
// 		pipe.ZAdd(ctx, eventKey, redis.Z{
// 			Score:  float64(eventId),
// 			Member: data,
// 		})
// 		pipe.Publish(ctx, eventKey, fmt.Sprintf("%d:%s", eventId, data))
// 		return nil
// 	})
// }
