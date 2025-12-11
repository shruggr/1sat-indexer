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
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/b-open-io/overlay/beef"
	"github.com/bsv-blockchain/arcade"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var JUNGLEBUS string
var Cache *redis.Client
var JB *junglebus.Client
var Chaintracks arcade.Chaintracks
var BeefStorage *beef.Storage

var ErrNotFound = errors.New("not-found")
var ErrBadRequest = errors.New("bad-request")
var ErrMalformed = errors.New("malformed")

func init() {
	godotenv.Load(".env")
	var err error
	JUNGLEBUS = os.Getenv("JUNGLEBUS")
	if JUNGLEBUS == "" {
		JUNGLEBUS = "https://junglebus.gorillapool.io"
	}
	log.Println("JUNGLEBUS", JUNGLEBUS)
	JB, err = junglebus.New(
		junglebus.WithHTTP(JUNGLEBUS),
	)
	if err != nil {
		log.Panic(err)
	}

	// Initialize BEEF storage with Redis + JungleBus fallback
	beefConn := os.Getenv("BEEF_URL")
	if beefConn == "" {
		// Build default connection string: lru -> redis -> junglebus
		beefConn = "lru://?size=100mb,~/.1sat/beef,junglebus://"
	}
	log.Println("BEEF_URL", beefConn)
	// Note: Chaintracks may not be initialized yet in init(), so we pass nil
	// The caller should call InitBeefStorage after Chaintracks is ready
	BeefStorage, err = beef.NewStorage(beefConn, nil)
	if err != nil {
		log.Panic(err)
	}
}

func LoadTx(ctx context.Context, txid string, withProof bool) (tx *transaction.Transaction, err error) {
	hash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, err
	}

	// Load raw tx from storage chain
	rawTx, err := BeefStorage.LoadRawTx(ctx, hash)
	if err != nil {
		return nil, ErrNotFound
	}

	tx, err = transaction.NewTransactionFromBytes(rawTx)
	if err != nil {
		return nil, ErrMalformed
	}

	if withProof {
		// Try to load proof (optional - tx may be unmined)
		if proofBytes, err := BeefStorage.LoadProof(ctx, hash); err == nil && len(proofBytes) > 0 {
			if proof, err := transaction.NewMerklePathFromBinary(proofBytes); err == nil {
				tx.MerklePath = proof
			}
		}
	}

	return tx, nil
}

func LoadRawtx(ctx context.Context, txid string) (rawtx []byte, err error) {
	hash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, err
	}
	return BeefStorage.LoadRawTx(ctx, hash)
}

func LoadProof(ctx context.Context, txid string) (proof *transaction.MerklePath, err error) {
	hash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, err
	}

	proofBytes, err := BeefStorage.LoadProof(ctx, hash)
	if err != nil {
		return nil, err
	}

	proof, err = transaction.NewMerklePathFromBinary(proofBytes)
	if err != nil {
		return nil, err
	}

	return proof, nil
}

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
	hash, err := chainhash.NewHashFromHex(txid)
	if err != nil {
		return nil, err
	}

	// Use overlay's LoadBeef which handles recursive input loading
	beefBytes, err := BeefStorage.LoadBeef(ctx, hash)
	if err != nil {
		return nil, err
	}

	return BeefStorage.LoadTxFromBeef(ctx, beefBytes, hash)
}

// Legacy helper functions for cache key compatibility
func TxKey(txid string) string {
	return "tx:" + txid
}

func ProofKey(txid string) string {
	return "prf:" + txid
}

// CacheTx caches a transaction directly to Redis (for broadcast scenarios)
func CacheTx(ctx context.Context, txid string, rawTx []byte) error {
	return Cache.Set(ctx, TxKey(txid), rawTx, 0).Err()
}

// CacheProof caches a proof directly to Redis with TTL
func CacheProof(ctx context.Context, txid string, proofBytes []byte, ttl time.Duration) error {
	return Cache.Set(ctx, ProofKey(txid), proofBytes, ttl).Err()
}
