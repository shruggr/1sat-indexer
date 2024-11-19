package idx

import (
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
	if opts, err := redis.ParseURL(os.Getenv("REDISTXO")); err != nil {
		panic(err)
	} else {
		TxoDB = redis.NewClient(opts)
	}

	log.Println("REDISQUEUE", os.Getenv("REDISQUEUE"))
	if opts, err := redis.ParseURL(os.Getenv("REDISQUEUE")); err != nil {
		panic(err)
	} else {
		QueueDB = redis.NewClient(opts)
	}

	log.Println("REDISACCT", os.Getenv("REDISACCT"))
	if opts, err := redis.ParseURL(os.Getenv("REDISACCT")); err != nil {
		panic(err)
	} else {
		AcctDB = redis.NewClient(opts)
	}
}

// const IngestLogKey = "log:tx"
// const IngestQueueKey = "que:ing"

// const PAGE_SIZE = 1000

// func Delog(ctx context.Context, tag string, id string) error {
// 	return QueueDB.ZRem(ctx, LogKey(tag), id).Err()
// }

// func Log(ctx context.Context, tag string, id string, score float64) (err error) {
// 	return QueueDB.ZAdd(ctx, LogKey(tag), redis.Z{
// 		Score:  score,
// 		Member: id,
// 	}).Err()
// }

// func LogScore(ctx context.Context, tag string, id string) (score float64, err error) {
// 	if score, err = QueueDB.ZScore(ctx, LogKey(tag), id).Result(); err == redis.Nil {
// 		err = nil
// 	}
// 	return
// }

// func Enqueue(ctx context.Context, tag string, id string, score float64) error {
// 	return QueueDB.ZAdd(ctx, QueueKey(tag), redis.Z{
// 		Score:  score,
// 		Member: id,
// 	}).Err()
// }

// func Dequeue(ctx context.Context, tag string, id string) error {
// 	return QueueDB.ZRem(ctx, QueueKey(tag), id).Err()
// }
