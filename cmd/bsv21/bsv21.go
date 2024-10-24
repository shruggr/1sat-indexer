package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/onesat"
)

var PAGE_SIZE = uint32(1000)

var CONCURRENCY uint
var ctx = context.Background()

var ingest = &idx.IngestCtx{
	Tag: "bsv21",
	Indexers: []idx.Indexer{
		&onesat.InscriptionIndexer{},
		&onesat.Bsv21Indexer{},
		&onesat.OrdLockIndexer{},
	},
	Network:     config.Network,
	Concurrency: 1,
	PageSize:    PAGE_SIZE,
}

func main() {
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	flag.Parse()

	limiter := make(chan struct{}, CONCURRENCY)

	if tokenIds, err := idx.Search(ctx, &idx.SearchCfg{
		Key: evt.EventKey(onesat.BSV21_TAG, &evt.Event{
			Id:    "issue",
			Value: "",
		}),
		Limit: 0,
	}); err != nil {
		log.Panic(err)
	} else {
		for _, tokenId := range tokenIds {
			limiter <- struct{}{}
			go func(tokenId string) {
				defer func() { <-limiter }()
				if err := processToken(tokenId); err != nil {
					log.Println("Error processing token", tokenId, err)
				}
			}(tokenId)
		}
		if len(tokenIds) == 0 {
			// log.Println("No results")
			time.Sleep(time.Second)
		}
	}
}

func processToken(tokenId string) (err error) {
	if tokenTxo, err := idx.LoadTxo(ctx, tokenId, []string{onesat.BSV21_TAG}); err != nil {
		log.Println("Error loading token", tokenId, err)
		return err
	} else if tokenTxo == nil {
		log.Println("Missing token", tokenId, err)
		return err
	} else if bsv21Data, ok := tokenTxo.Data[onesat.BSV21_TAG]; !ok {
		log.Println("Missing BSV21 data", tokenId)
		return err
	} else {
		bsv21 := bsv21Data.Data.(*onesat.Bsv21)
		if bsv21.FundAddress == "" {
			idx.AcctDB.ZAddNX(ctx, idx.OwnerSyncKey, redis.Z{
				Score:  0,
				Member: bsv21.FundAddress,
			})
		}
		pendingKey := evt.EventKey(onesat.BSV21_TAG, &evt.Event{
			Id:    onesat.PendingEvent,
			Value: bsv21.Id,
		})
		if pendingCount, err := idx.TxoDB.ZCard(ctx, pendingKey).Result(); err != nil {
			log.Println("Error counting pending", bsv21.FundAddress, err)
			return err
		} else if pendingCount == 0 {
			return nil
		}

		if balance, err := idx.Balance(ctx, idx.OwnerTxosKey(bsv21.FundAddress)); err != nil {
			log.Println("Error getting balance", tokenId, bsv21.FundAddress, err)
			return err
		} else if validCount, err := idx.TxoDB.ZCard(ctx, evt.EventKey(onesat.BSV21_TAG, &evt.Event{
			Id:    onesat.ValidEvent,
			Value: bsv21.Id,
		})).Uint64(); err != nil {
			log.Println("Error counting valid", tokenId, bsv21.FundAddress, err)
			return err
		} else if invalidCount, err := idx.TxoDB.ZCard(ctx, evt.EventKey(onesat.BSV21_TAG, &evt.Event{
			Id:    onesat.InvalidEvent,
			Value: bsv21.Id,
		})).Uint64(); err != nil {
			log.Println("Error counting invalid", tokenId, bsv21.FundAddress, err)
			return err
		} else {
			validationCount := validCount + invalidCount
			validationCost := validationCount * onesat.BSV21_INDEX_FEE
			if balance < validationCost {
				log.Println("Insufficient balance", tokenId, bsv21.FundAddress, balance, validationCost)
				return nil
			}
			limit := uint32((balance - uint64(validationCost)) / onesat.BSV21_INDEX_FEE)
			if limit == 0 {
				log.Println("Insufficient balance", tokenId, bsv21.FundAddress, balance, validationCost)
				return nil
			}
			tokenIngest := *ingest
			tokenIngest.Limit = limit
			onIngest := func(ctx context.Context, idxCtx *idx.IndexContext) error {
				for _, txo := range idxCtx.Txos {
					if idxData, ok := txo.Data[onesat.BSV21_TAG]; ok {
						if bsv21, ok := idxData.Data.(*onesat.Bsv21); ok {
							if bsv21.Id != tokenId || bsv21.Status == onesat.Pending {
								continue
							}
							idx.TxoDB.ZRem(ctx, pendingKey, txo.Outpoint.String())
						}
					}
				}
				return nil
			}
			tokenIngest.OnIngest = &onIngest
			if err := tokenIngest.Exec(ctx); err != nil {
				log.Println("Error ingesting token", tokenId, err)
				return err
			}
		}
	}
	return nil
}
