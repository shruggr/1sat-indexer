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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
)

var TRIGGER = uint32(783968)

var Db *pgxpool.Pool

var Rdb *redis.Client
var Cache *redis.Client
var JB *junglebus.Client
var bit *bitcoin.Bitcoind

func Initialize(postgres *pgxpool.Pool, rdb *redis.Client, cache *redis.Client) (err error) {
	Db = postgres
	Rdb = rdb
	Cache = cache

	log.Println("JUNGLEBUS", os.Getenv("JUNGLEBUS"))
	if os.Getenv("JUNGLEBUS") != "" {
		JB, err = junglebus.New(
			junglebus.WithHTTP(os.Getenv("JUNGLEBUS")),
		)
		if err != nil {
			return
		}
	}

	if os.Getenv("BITCOIN_HOST") != "" {
		port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
		bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
		if err != nil {
			log.Panic(err)
		}
	}

	return
}

func LoadTx(txid string) (tx *bt.Tx, err error) {
	rawtx, err := LoadRawtx(txid)
	if err != nil {
		return
	}
	if len(rawtx) == 0 {
		err = fmt.Errorf("missing-txn %s", txid)
		return
	}
	return bt.NewTxFromBytes(rawtx)
}

func LoadRawtx(txid string) (rawtx []byte, err error) {
	rawtx, _ = Cache.HGet(context.Background(), "tx", txid).Bytes()

	if len(rawtx) > 100 {
		return rawtx, nil
	} else {
		rawtx = []byte{}

	}

	if len(rawtx) == 0 && JB != nil {
		url := fmt.Sprintf("%s/v1/transaction/get/%s/bin", os.Getenv("JUNGLEBUS"), txid)

		if resp, err := http.Get(url); err == nil && resp.StatusCode < 300 {
			rawtx, _ = io.ReadAll(resp.Body)
		}
	}

	if len(rawtx) == 0 && bit != nil {
		// log.Println("Requesting tx from node", txid)
		if r, err := bit.GetRawTransactionRest(txid); err == nil {
			rawtx, _ = io.ReadAll(r)
		}
	}

	if len(rawtx) == 0 {
		err = fmt.Errorf("missing-txn %s", txid)
		return
	}

	Cache.HSet(context.Background(), "tx", txid, rawtx).Err()
	return
}

func LoadTxOut(outpoint *Outpoint) (txout *bt.Output, err error) {
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
	txout = &bt.Output{}
	_, err = txout.ReadFrom(resp.Body)
	return
}

func GetSpend(outpoint *Outpoint) (spend []byte, err error) {
	resp, err := http.Get(fmt.Sprintf("%s/v1/txo/spend/%s", os.Getenv("JUNGLEBUS"), outpoint.String()))
	if err != nil {
		log.Println("JB Spend Request", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		err = fmt.Errorf("missing-spend-%s", outpoint.String())
		return
	}
	return io.ReadAll(resp.Body)
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

func PublishEvent(ctx context.Context, event string, data string) {
	eventId := time.Now().Unix()
	cutoff := time.Now().Add(-24 * time.Hour)
	eventKey := "evt:" + event
	Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZRemRangeByScore(ctx, eventKey, "-inf", fmt.Sprintf("%d", cutoff.Unix()))
		pipe.ZAdd(ctx, eventKey, redis.Z{
			Score:  float64(eventId),
			Member: data,
		})
		pipe.Publish(ctx, eventKey, fmt.Sprintf("%d:%s", eventId, data))
		return nil
	})
}
