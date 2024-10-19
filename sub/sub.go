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
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/jb"
)

const ProgressKey = "progress"

type Sub struct {
	Tag          string
	IndexBlocks  bool
	IndexMempool bool
	Topic        string
	FromBlock    uint
	Verbose      bool
}

func (cfg *Sub) Exec(ctx context.Context) (err error) {
	errors := make(chan error)

	progressKey := ProgressKey

	var sub *junglebus.Subscription

	eventHandler := junglebus.EventHandler{
		OnStatus: func(status *models.ControlResponse) {
			// if cfg.Verbose {
			log.Printf("[STATUS]: %d %v\n", status.StatusCode, status.Message)
			// }
			if status.StatusCode == 200 {
				if err := evt.DB.ZAdd(ctx, progressKey, redis.Z{
					Score:  float64(status.Block),
					Member: cfg.Tag,
				}).Err(); err != nil {
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
			if err := idx.QueneTx(ctx, txn.Id, idx.HeightScore(txn.BlockHeight, txn.BlockIndex)); err != nil {
				errors <- err
			}
		}
	}
	if cfg.IndexMempool {
		eventHandler.OnMempool = func(txn *models.TransactionResponse) {
			if cfg.Verbose {
				log.Printf("[MEMPOOL]: %d %s\n", len(txn.Transaction), txn.Id)
			}
			if err := idx.QueneTx(ctx, txn.Id, idx.HeightScore(uint32(time.Now().Unix()), 0)); err != nil {
				errors <- err
			}
		}
	}

	if progress, err := evt.DB.ZScore(ctx, progressKey, cfg.Tag).Result(); err != nil && err != redis.Nil {
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
