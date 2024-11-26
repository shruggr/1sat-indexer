package sub

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

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
}

var store *redisstore.RedisStore

func init() {
	store = redisstore.NewRedisTxoStore(os.Getenv("REDISTXO"), os.Getenv("REDISACCT"), os.Getenv("REDISQUEUE"))
}

func (cfg *Sub) Exec(ctx context.Context) (err error) {
	errors := make(chan error)

	var sub *junglebus.Subscription

	eventHandler := junglebus.EventHandler{
		OnStatus: func(status *models.ControlResponse) {
			// if cfg.Verbose {
			log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
			// }
			if status.StatusCode == 200 {
				if err := store.Log(ctx, ProgressKey, cfg.Tag, float64(status.Block)); err != nil {
					errors <- err
				}
			} else if status.StatusCode == 999 {
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
			if cfg.Verbose {
				log.Printf("[TX]: %d - %d: %d %s\n", txn.BlockHeight, txn.BlockIndex, len(txn.Transaction), txn.Id)
			}
			if err := store.Log(ctx, cfg.Queue, txn.Id, idx.HeightScore(txn.BlockHeight, txn.BlockIndex)); err != nil {
				errors <- err
			}
		}
	}
	if cfg.IndexMempool {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			if cfg.Verbose {
				log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			}
			if err := store.Log(ctx, cfg.Queue, txn.Id, idx.HeightScore(0, 0)); err != nil {
				errors <- err
			}
		}
	}

	if progress, err := store.LogScore(ctx, ProgressKey, cfg.Tag); err != nil {
		log.Panic(err)
	} else if progress > 6 {
		cfg.FromBlock = uint(progress) - 5
	}
	log.Println("Subscribing to Junglebus from block", cfg.FromBlock)
	if sub, err = jb.JB.SubscribeWithQueue(ctx,
		cfg.Topic,
		uint64(cfg.FromBlock),
		0,
		eventHandler,
		&junglebus.SubscribeOptions{
			QueueSize: 1000,
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
