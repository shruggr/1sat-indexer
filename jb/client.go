package jb

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/joho/godotenv"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/blk"
)

var JUNGLEBUS string
var Cache *redis.Client
var JB *junglebus.Client
var bit *bitcoin.Bitcoind

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))
	var err error
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
	if JUNGLEBUS != "" {
		log.Println("JUNGLEBUS", JUNGLEBUS)
		JB, err = junglebus.New(
			junglebus.WithHTTP(JUNGLEBUS),
		)
		if err != nil {
			log.Panic(err)
		}
	}

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
}

func TxKey(txid string) string {
	return "tx:" + txid
}

func ProofKey(txid string) string {
	return "prf:" + txid
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
		if proof, err := LoadProof(ctx, txid); err == nil && proof != nil {
			tx.MerklePath = proof
		}
	}

	return
}

type InFlight struct {
	Result []byte
	Wg     sync.WaitGroup
}

var inflightMap = map[string]*InFlight{}
var inflightM sync.Mutex

func LoadRawtx(ctx context.Context, txid string) (rawtx []byte, err error) {
	cacheKey := TxKey(txid)
	rawtx, _ = Cache.Get(ctx, cacheKey).Bytes()

	if len(rawtx) > 100 {
		return rawtx, nil
	} else {
		rawtx = []byte{}

	}

	if len(rawtx) == 0 && JB != nil {
		// start := time.Now()
		url := fmt.Sprintf("%s/v1/transaction/get/%s/bin", os.Getenv("JUNGLEBUS"), txid)
		// fmt.Println("Requesting:", url)
		inflightM.Lock()
		inflight, ok := inflightMap[url]
		if !ok {
			inflight = &InFlight{}
			inflight.Wg.Add(1)
			inflightMap[url] = inflight
		}
		inflightM.Unlock()
		if ok {
			// log.Println("Waiting for rawtx", txid)
			inflight.Wg.Wait()
			rawtx = inflight.Result
		} else {
			// log.Println("Requesting rawtx", txid)
			if resp, err := http.Get(url); err == nil && resp.StatusCode < 300 {
				rawtx, _ = io.ReadAll(resp.Body)
			}
			inflight.Result = rawtx
			inflight.Wg.Done()

			inflightM.Lock()
			delete(inflightMap, url)
			inflightM.Unlock()
		}
		// log.Println("Rawtx", txid, time.Since(start))
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
		// start := time.Now()
		url := fmt.Sprintf("%s/v1/transaction/proof/%s/bin", os.Getenv("JUNGLEBUS"), txid)
		inflightM.Lock()
		inflight, ok := inflightMap[url]
		if !ok {
			inflight = &InFlight{}
			inflight.Wg.Add(1)
			inflightMap[url] = inflight
		}
		inflightM.Unlock()
		if ok {
			// log.Println("Waiting for proof", txid)
			inflight.Wg.Wait()
			prf = inflight.Result
		} else {
			// log.Println("Requesting proof", txid)
			if resp, err := http.Get(url); err == nil && resp.StatusCode < 300 {
				prf, _ = io.ReadAll(resp.Body)
			}
			inflight.Result = prf
			inflight.Wg.Done()

			inflightM.Lock()
			delete(inflightMap, url)
			inflightM.Unlock()
		}

		// log.Println("Proof", txid, time.Since(start))
	}
	if len(prf) > 0 {
		if proof, err = transaction.NewMerklePathFromBinary(prf); err != nil {
			return
		} else if chaintip, err := blk.Chaintip(ctx); err != nil {
			return nil, err
		} else if proof.BlockHeight < uint32(chaintip.Height) {
			Cache.Set(ctx, cacheKey, prf, 0)
		} else {
			Cache.Set(ctx, cacheKey, prf, time.Hour)
		}

	}
	return
}

// func LoadTxOut(outpoint *lib.Outpoint) (txout *transaction.TransactionOutput, err error) {
// 	url := fmt.Sprintf("https://junglebus.gorillapool.io/v1/txo/get/%s", outpoint.String())
// 	// log.Println("Requesting txo", url)
// 	resp, err := http.Get(url)
// 	if err != nil {
// 		return
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != 200 {
// 		err = fmt.Errorf("missing-txn %s", outpoint.String())
// 		return
// 	}
// 	txout = &transaction.TransactionOutput{}
// 	_, err = txout.ReadFrom(resp.Body)
// 	return
// }

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
