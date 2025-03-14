package jb

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/joho/godotenv"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/blk"
)

var JUNGLEBUS string
var Cache *redis.Client
var JB *junglebus.Client
var bit *bitcoin.Bitcoind

var ErrNotFound = errors.New("not-found")
var ErrBadRequest = errors.New("bad-request")
var ErrMalformed = errors.New("malformed")

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
	if opts, err := redis.ParseURL(os.Getenv("REDISCACHE")); err != nil {
		panic(err)
	} else {
		Cache = redis.NewClient(opts)
	}

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
	cacheKey := TxKey(txid)
	rawtx, _ = Cache.Get(ctx, cacheKey).Bytes()
	fromCache := false
	if len(rawtx) > 0 {
		if tx, err = transaction.NewTransactionFromBytes(rawtx); err != nil {
			log.Println("fixing bad cache", txid)
			Cache.Del(ctx, cacheKey)
			err = ErrMalformed
			tx = nil
		} else {
			fromCache = true
		}
	}

	if tx == nil && JB != nil {
		if rawtx, err = LoadRemoteRawtx(ctx, txid); err != nil {
			log.Println("JB Err", txid, err)
		} else if len(rawtx) > 0 {
			if tx, err = transaction.NewTransactionFromBytes(rawtx); err != nil {
				log.Println("JB Malformed", txid, err)
				err = ErrMalformed
				tx = nil
			} else {
				// log.Println("JB Success", txid)
			}
		} else {
			log.Println("JB Missing", txid)
		}
	}

	if tx == nil && bit != nil {
		// log.Println("Requesting tx from node", txid)
		var r io.ReadCloser
		if r, err = bit.GetRawTransactionRest(txid); err != nil {
			log.Println("node error", txid, err)
		} else if rawtx, err = io.ReadAll(r); err != nil {
			log.Println("read error", txid, err)
		} else if len(rawtx) > 0 {
			if tx, err = transaction.NewTransactionFromBytes(rawtx); err != nil {
				err = ErrMalformed
			}
		}
	}
	if err != nil {
		return
	}
	if tx == nil {
		err = ErrNotFound
		return
	}

	if !fromCache {
		Cache.Set(ctx, cacheKey, rawtx, 0)
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
	if tx, err := LoadTx(ctx, txid, false); err != nil {
		return nil, err
	} else {
		return tx.Bytes(), nil
	}
}

func LoadRemoteRawtx(ctx context.Context, txid string) (rawtx []byte, err error) {
	// start := time.Now()
	url := fmt.Sprintf("%s/v1/transaction/get/%s/bin", JUNGLEBUS, txid)
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
		rawtx, err = func() (rawtx []byte, err error) {
			defer func() {
				inflight.Result = rawtx
				inflight.Wg.Done()

				inflightM.Lock()
				delete(inflightMap, url)
				inflightM.Unlock()
			}()
			if resp, err := http.Get(url); err != nil {
				return nil, err
			} else if resp.StatusCode == 404 {
				return nil, ErrNotFound
			} else if resp.StatusCode != 200 {
				return nil, fmt.Errorf("%d %s", resp.StatusCode, rawtx)
			} else if rawtx, err = io.ReadAll(resp.Body); err != nil {
				return nil, err
			} else {
				return rawtx, nil
			}
		}()

	}
	// log.Println("Rawtx", txid, time.Since(start))
	return
}

func LoadProof(ctx context.Context, txid string) (proof *transaction.MerklePath, err error) {
	cacheKey := ProofKey(txid)
	prf, _ := Cache.Get(ctx, cacheKey).Bytes()
	if len(prf) == 0 && JB != nil {
		// start := time.Now()
		url := fmt.Sprintf("%s/v1/transaction/proof/%s/bin", JUNGLEBUS, txid)
		// log.Println("Requesting:", url)
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
		} else if chaintip, err := blk.GetChaintip(ctx); err != nil {
			return nil, err
		} else if proof.BlockHeight+5 < uint32(chaintip.Height) {
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

func GetSpend(outpoint string) (spend string, err error) {
	url := fmt.Sprintf("%s/v1/txo/spend/%s", JUNGLEBUS, outpoint)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("JB Spend Request", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err = fmt.Errorf("missing-spend-%s", outpoint)
		return
	}
	if b, err := io.ReadAll(resp.Body); err != nil {
		return "", err
	} else {
		spend := hex.EncodeToString(b)
		return spend, nil
	}
}

func BuildTxBEEF(ctx context.Context, txid string) (tx *transaction.Transaction, err error) {
	log.Println("Building BEEF", txid)
	if tx, err = LoadTx(ctx, txid, true); err != nil {
		return nil, err
	} else if tx.MerklePath == nil {
		for _, in := range tx.Inputs {
			if in.SourceTransaction == nil {
				sourceTxid := in.SourceTXID.String()
				log.Println("Recursing BEEF", sourceTxid, "from", txid)
				if in.SourceTransaction, err = BuildTxBEEF(ctx, sourceTxid); err != nil {
					return nil, err
				}
			}
		}
	}
	return tx, nil
}
