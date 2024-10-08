package lib

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/redis/go-redis/v9"
)

type Event struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

type IndexData struct {
	Data     any         `json:"data"`
	Events   []*Event    `json:"events"`
	FullText string      `json:"text"`
	Deps     []*Outpoint `json:"deps"`
	Validate bool        `json:"validate"`
}

type IndexContext struct {
	Tx     *transaction.Transaction `json:"-"`
	Txid   *ByteString              `json:"txid"`
	Height uint32                   `json:"height"`
	Idx    uint64                   `json:"idx"`
	Txos   []*Txo                   `json:"txos"`
	Spends []*Txo                   `json:"spends"`
}

func (idxCtx *IndexContext) Score() float64 {
	score, _ := strconv.ParseFloat(fmt.Sprintf("%07d.%09d", idxCtx.Height, idxCtx.Idx), 64)
	return score
}

func (idxCtx *IndexContext) Save(ctx context.Context) {
	txid := hex.EncodeToString(*idxCtx.Txid)
	if err := Rdb.ZAdd(ctx, "status", redis.Z{
		Score:  float64(idxCtx.Height / 10000000),
		Member: txid,
	}).Err(); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
	} else if _, err := Db.Exec(context.Background(), `
		INSERT INTO txns(txid, height, idx)
		VALUES($1, $2, $3)
		ON CONFLICT(txid) DO NOTHING`,
		idxCtx.Txid,
		idxCtx.Height,
		idxCtx.Idx,
	); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
		// } else if err := Rdb.ZAdd(ctx, "processed", redis.Z{
		// 	Score:  float64(time.Now().Unix()),
		// 	Member: idxCtx.Txid,
		// }).Err(); err != nil {
		// 	log.Panicf("%x %v\n", idxCtx.Txid, err)
	}
	limiter := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, spend := range idxCtx.Spends {
		limiter <- struct{}{}
		wg.Add(1)
		go func(spend *Txo) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			if err := spend.SaveSpend(ctx); err != nil {
				log.Panic(err)
			}
		}(spend)
	}
	for _, txo := range idxCtx.Txos {
		limiter <- struct{}{}
		wg.Add(1)
		go func(txo *Txo) {
			defer func() {
				<-limiter
				wg.Done()
			}()
			if err := txo.Save(ctx); err != nil {
				log.Panic(err)
			}
			// if Rdb != nil && txo.Owner != nil {
			// 	if address, err := txo.Owner.Address(); err == nil {
			// 		PublishEvent(context.Background(), address, txo.Outpoint.String())
			// 	}
			// }
		}(txo)
	}
	wg.Wait()
	status := 1
	if idxCtx.Height > 0 && idxCtx.Height < 50000000 {
		status = 2
	}
	if err := Rdb.ZAdd(ctx, "status", redis.Z{
		Score:  float64(status) + float64(idxCtx.Height/10000000),
		Member: txid,
	}).Err(); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
	}

}

// func (idxCtx *IndexContext) SaveSpends(ctx context.Context) error {
// 	outpoints := make([][]byte, 0, len(idxCtx.Spends))
// 	for _, spend := range idxCtx.Spends {
// 		outpoints = append(outpoints, *spend.Outpoint)
// 	}

// 	_, err := Db.Exec(ctx, `UPDATE txos
// 		SET spend=$1, spend_height=$2, spend_idx=$3
// 		WHERE outpoint=ANY($4)`,
// 		idxCtx.Txid,
// 		idxCtx.Height,
// 		idxCtx.Idx,
// 		outpoints,
// 	)
// 	return err
// }

// func (idxCtx *IndexContext) Save() error {
// 	if err := Rdb.ZAdd(context.Background(), "txns", redis.Z{
// 		Score:  idxCtx.Score(),
// 		Member: hex.EncodeToString(idxCtx.Txid),
// 	}).Err(); err != nil {
// 		return err
// 	}
// 	outpoints := make([]string, len(idxCtx.Txos))
// 	for _, txo := range idxCtx.Txos {
// 		outpoints = append(outpoints, txo.Outpoint.String())
// 	}
// 	score := idxCtx.Score()
// 	var spends map[string]string
// 	err := Rdb.HMGet(ctx, "spends", outpoints...).Scan(&spends)
// 	if err != nil {
// 		return err
// 	}
// 	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
// 		for _, txo := range idxCtx.Txos {
// 			outpoint := txo.Outpoint.String()
// 			pipe.ZAdd(ctx, "txos", redis.Z{
// 				Score:  score,
// 				Member: outpoint,
// 			})
// 			if txo.Owner == nil {
// 				continue
// 			}
// 			if add, err := txo.Owner.Address(); err != nil {
// 				continue
// 			} else if err := pipe.ZAdd(ctx, "add:"+add, redis.Z{
// 				Score:  score,
// 				Member: outpoint,
// 			}).Err(); err != nil {
// 				return err
// 			} else if err := pipe.SAdd(ctx, "own:"+outpoint, add).Err(); err != nil {
// 				return err
// 			} else if spend, ok := spends[outpoint]; ok {
// 				if score, err := pipe.ZScore(ctx, "txns", spend).Result(); err != nil {
// 					return err
// 				} else if err := pipe.ZAdd(ctx, "add:"+add, redis.Z{
// 					Score:  score,
// 					Member: spend,
// 				}).Err(); err != nil {
// 					return err
// 				}
// 			}
// 		}
// 		return nil
// 	}); err != nil {
// 		return err
// 	}

// 	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
// 		for _, spend := range idxCtx.Spends {
// 			outpoint := spend.Outpoint.String()
// 			if err := pipe.HSet(ctx, "spends", outpoint, hex.EncodeToString(idxCtx.Txid)).Err(); err != nil {
// 				return err
// 			} else if err := pipe.SAdd(ctx, "own:"+outpoint, add).Err(); err != nil {
// 				return err
// 			} else if spend, ok := spends[outpoint]; ok {
// 				if score, err := pipe.ZScore(ctx, "txns", spend).Result(); err != nil {
// 					return err
// 				} else if err := pipe.ZAdd(ctx, "add:"+add, redis.Z{
// 					Score:  score,
// 					Member: spend,
// 				}).Err(); err != nil {
// 					return err
// 				}
// 			}
// 		}
// 		return nil
// 	}); err != nil {
// 		return err
// 	}

// 	// if err := Rdb.HSet(ctx, "spends", spend.Outpoint, idxCtx.Txid).Err(); err != nil {
// 	// 	return err
// 	// }
// 	// if _, err := Rdb.Pipelined(context.Background(), func(pipe redis.Pipeliner) error {
// 	// 	for _, txo := range idxCtx.Txos {
// 	// 		if add, err := txo.PKHash.Address(); err != nil {
// 	// 			log.Panic(err)
// 	// 		} else if err := pipe.ZAdd(ctx, "txos:"+add, redis.Z{
// 	// 			Score:  score,
// 	// 			Member: txo.Outpoint,
// 	// 		}).Err(); err != nil {
// 	// 			return err
// 	// 		}
// 	// 	}

// 	// 	return nil
// 	// }); err != nil {
// 	// 	log.Panic(err)
// 	// }
// }

// // func (ctx *IndexContext) SaveSpends() {
// // 	limiter := make(chan struct{}, 32)
// // 	var wg sync.WaitGroup

// // 	for _, spend := range ctx.Spends {
// // 		limiter <- struct{}{}
// // 		wg.Add(1)
// // 		go func(spend *Txo) {
// // 			defer func() {
// // 				<-limiter
// // 				wg.Done()
// // 			}()
// // 			spend.SaveSpend()
// 		}(spend)
// 	}
// 	wg.Wait()
// }