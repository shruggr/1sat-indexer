package sub

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	"github.com/shruggr/1sat-indexer/v5/idx"
	redisstore "github.com/shruggr/1sat-indexer/v5/idx/redis-store"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

const ProgressKey = "progress"

type Sub struct {
	Tag          string
	Queue        string
	IndexBlocks  bool
	IndexMempool bool
	Topic        string
	FromBlock    uint
	Verbose      bool
	ReorgRewind  bool
}

var store *redisstore.RedisStore

func init() {
	var err error
	if store, err = redisstore.NewRedisStore(os.Getenv("REDISTXO")); err != nil {
		panic(err)
	}
}

func (cfg *Sub) Exec(ctx context.Context) (err error) {
	errors := make(chan error)
	queueKey := idx.QueueKey(cfg.Queue)
	var sub *junglebus.Subscription
	txcount := 0
	logs := make([]idx.Log, 0, 100000)
	lastActivity := time.Now()

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		for range ticker.C {
			if time.Since(lastActivity) > 15*time.Minute {
				log.Println("No activity for 15 minutes, exiting...")
				os.Exit(0)
			}
		}
	}()

	eventHandler := junglebus.EventHandler{
		OnStatus: func(status *models.ControlResponse) {
			lastActivity = time.Now()
			// if cfg.Verbose {
			log.Printf("[STATUS]: %d %v %d processed\n", status.StatusCode, status.Message, txcount)
			// }
			switch status.StatusCode {
			case 199:
				if len(logs) > 0 {
					if err := store.LogMany(ctx, queueKey, logs); err != nil {
						errors <- err
					}
					logs = make([]idx.Log, 0, 100000)
				}
			case 200:
				if len(logs) > 0 {
					if err := store.LogMany(ctx, queueKey, logs); err != nil {
						errors <- err
					}
					logs = make([]idx.Log, 0, 100000)
				}
				if err := store.Log(ctx, ProgressKey, cfg.Tag, float64(status.Block)); err != nil {
					errors <- err
				}
				txcount = 0
			case 999:
				log.Println(status.Message)
				log.Println("Unsubscribing...")
				sub.Unsubscribe()
				os.Exit(0)
				return
			}
		},
		OnError: func(err error) {
			log.Panicf("[ERROR]: %v\n", err)
		},
	}

	if cfg.IndexBlocks {
		eventHandler.OnTransaction = func(txn *models.TransactionResponse) {
			txcount++
			if cfg.Verbose {
				log.Printf("[TX]: %d - %d: %d %s\n", txn.BlockHeight, txn.BlockIndex, len(txn.Transaction), txn.Id)
			}
			logs = append(logs, idx.Log{
				Member: txn.Id,
				Score:  idx.HeightScore(txn.BlockHeight, txn.BlockIndex),
			})
			// if err := store.Log(ctx, queueKey, txn.Id, idx.HeightScore(txn.BlockHeight, txn.BlockIndex)); err != nil {
			// 	errors <- err
			// }
		}
	}
	if cfg.IndexMempool {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			if cfg.Verbose {
				log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			}
			if err := store.Log(ctx, queueKey, txn.Id, idx.HeightScore(0, 0)); err != nil {
				errors <- err
			}
		}
	}

	if progress, err := store.LogScore(ctx, ProgressKey, cfg.Tag); err != nil {
		log.Panic(err)
	} else if progress > 6 {
		if cfg.ReorgRewind {
			cfg.FromBlock = uint(progress) - 5
		} else {
			cfg.FromBlock = uint(progress)
		}
	}
	log.Println("Subscribing to Junglebus from block", cfg.FromBlock)
	if sub, err = jb.JB.SubscribeWithQueue(ctx,
		cfg.Topic,
		uint64(cfg.FromBlock),
		0,
		eventHandler,
		&junglebus.SubscribeOptions{
			QueueSize: 10000000,
			LiteMode:  true,
		},
	); err != nil {
		log.Panic(err)
	}
	defer func() {
		sub.Unsubscribe()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err = <-errors:
	case <-sigs:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return err

}
