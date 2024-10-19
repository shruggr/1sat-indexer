package idx

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/GorillaPool/go-junglebus"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var AcctDB *redis.Client
var TxoDB *redis.Client
var QueueDB *redis.Client
var JUNGLEBUS string
var JB *junglebus.Client

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

	log.Println("REDISTXO", os.Getenv("REDISTXO"))
	TxoDB = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISTXO"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	log.Println("REDISQUEUE", os.Getenv("REDISQUEUE"))
	QueueDB = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISQUEUE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	log.Println("REDISACCT", os.Getenv("REDISACCT"))
	AcctDB = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISACCT"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
}

const IngestLogKey = "log:tx"
const IngestQueueKey = "que:ing"
const TxosKey = "txos"
const SpendsKey = "spends"

func HeightScore(height uint32, idx uint64) float64 {
	return float64(uint64(height)*1000000000 + idx)
}

func TxoDataKey(outpoint string) string {
	return "txo:data:" + outpoint
}

const PAGE_SIZE = 1000

func TxIngested(ctx context.Context, txid string) (bool, error) {
	if err := QueueDB.ZScore(ctx, IngestLogKey, txid).Err(); err != nil && err != redis.Nil {
		return false, err
	} else {
		return err != redis.Nil, nil
	}
}

func QueneTx(ctx context.Context, txid string, score float64) error {
	if err := QueueDB.ZAdd(ctx, IngestQueueKey, redis.Z{
		Score:  score,
		Member: txid,
	}).Err(); err != nil {
		return err
	}
	return nil
}
