package lib

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/bitcoin-sv/go-sdk/chainhash"
	"github.com/bitcoin-sv/go-sdk/script"
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

func NewIndexContext(tx *transaction.Transaction, indexers []Indexer) *IndexContext {
	idxCtx := &IndexContext{
		Tx:       tx,
		Txid:     tx.TxID(),
		Indexers: indexers,
	}

	if tx.MerklePath != nil {
		idxCtx.Height = tx.MerklePath.BlockHeight
		for _, path := range tx.MerklePath.Path[0] {
			if idxCtx.Txid.IsEqual(path.Hash) {
				idxCtx.Idx = path.Offset
				break
			}
		}
	} else {
		idxCtx.Height = uint32(time.Now().Unix())
	}
	return idxCtx
}

type IndexContext struct {
	Tx       *transaction.Transaction `json:"-"`
	Txid     *chainhash.Hash          `json:"txid"`
	Height   uint32                   `json:"height"`
	Idx      uint64                   `json:"idx"`
	Txos     []*Txo                   `json:"txos"`
	Spends   []*Txo                   `json:"spends"`
	Indexers []Indexer                `json:"-"`
}

func (idxCtx *IndexContext) ParseTxn(ctx context.Context) {
	if !idxCtx.Tx.IsCoinbase() {
		idxCtx.ParseSpends(ctx)
	}

	idxCtx.ParseTxos()
}

func (idxCtx *IndexContext) ParseTxos() {
	accSats := uint64(0)
	for vout, txout := range idxCtx.Tx.Outputs {
		outpoint := NewOutpointFromHash(idxCtx.Txid, uint32(vout))
		txo := &Txo{
			Outpoint: outpoint,
			Satoshis: txout.Satoshis,
			OutAcc:   accSats,
			Data:     make(map[string]*IndexData),
		}
		if len(*txout.LockingScript) >= 25 && script.NewFromBytes((*txout.LockingScript)[:25]).IsP2PKH() {
			pkhash := PKHash((*txout.LockingScript)[3:23])
			txo.AddOwner(pkhash.Address())
		}
		idxCtx.Txos = append(idxCtx.Txos, txo)
		accSats += txout.Satoshis
		for _, indexer := range idxCtx.Indexers {
			if data := indexer.Parse(idxCtx, uint32(vout)); data != nil {
				txo.Data[indexer.Tag()] = data
			}
		}
	}
}

func (idxCtx *IndexContext) ParseSpends(ctx context.Context) {
	for _, txin := range idxCtx.Tx.Inputs {
		outpoint := NewOutpointFromHash(txin.SourceTXID, txin.SourceTxOutIndex)
		if spend, err := idxCtx.LoadTxo(ctx, outpoint); err != nil {
			log.Panic(err)
		} else {
			idxCtx.Spends = append(idxCtx.Spends, spend)
		}
	}
}

func (idxCtx *IndexContext) Save(ctx context.Context) error {
	txid := idxCtx.Txid.String()
	if err := Rdb.ZAdd(ctx, TxStatusKey, redis.Z{
		Score:  float64(idxCtx.Height / 10000000),
		Member: txid,
	}).Err(); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
		return err
	} else if err := idxCtx.SaveTxos(ctx); err != nil {
		log.Panic(err)
		return err
	} else if err := idxCtx.SaveSpends(ctx); err != nil {
		log.Panic(err)
		return err
	}

	status := 1
	if idxCtx.Height > 0 && idxCtx.Height < 50000000 {
		status = 2
	}
	if err := Rdb.ZAdd(ctx, TxStatusKey, redis.Z{
		Score:  float64(status) + float64(idxCtx.Height/10000000),
		Member: txid,
	}).Err(); err != nil {
		log.Panicf("%x %v\n", idxCtx.Txid, err)
		return err
	}
	return nil
}

func (idxCtx *IndexContext) LoadTxo(ctx context.Context, outpoint *Outpoint) (*Txo, error) {
	txo := &Txo{}
	if j, err := Rdb.JSONGet(ctx, TxoKey(outpoint), "").Result(); err == redis.Nil || j == "" {
		if tx, err := LoadTx(ctx, outpoint.TxidHex()); err != nil {
			log.Panicln(err)
			return nil, err
		} else {
			spendCtx := NewIndexContext(tx, idxCtx.Indexers)
			spendCtx.ParseTxos()
			if err := spendCtx.SaveTxos(ctx); err != nil {
				log.Panic(err)
				return nil, err
			}
			return spendCtx.Txos[outpoint.Vout()], nil
		}
	} else if err != nil {
		log.Panicln(err)
		return nil, err
	} else if err = json.Unmarshal([]byte(j), txo); err != nil {
		log.Println("JSON", j)
		log.Panicln(err)
		return nil, err
	} else {
		return txo, nil
	}
}

func (idxCtx *IndexContext) SaveTxos(ctx context.Context) error {
	for vout := range idxCtx.Txos {
		if err := idxCtx.SaveTxo(ctx, uint32(vout)); err != nil {
			return err
		}
	}
	return nil
}

func (idxCtx *IndexContext) SaveTxo(ctx context.Context, vout uint32) error {
	txo := idxCtx.Txos[vout]
	spendKey := SpendKey(txo.Outpoint)
	outpoint := txo.Outpoint.String()
	score := float64(idxCtx.Height) + (float64(idxCtx.Idx) / 1000000000)

	accts := map[string]struct{}{}
	if len(txo.Owners) > 0 {
		if result, err := Rdb.HMGet(ctx, OwnerAccountKey, txo.Owners...).Result(); err != nil {
			log.Panic(err)
			return err
		} else {
			for _, item := range result {
				if acct, ok := item.(string); ok && acct != "" {
					accts[acct] = struct{}{}
				}
			}
		}
	}
	if err := Rdb.Watch(ctx, func(tx *redis.Tx) error {
		if spendVal, err := tx.Get(ctx, spendKey).Bytes(); err != nil && err != redis.Nil {
			return err
		} else if err != redis.Nil {
			if _, score, err = ParseSpendValue(spendVal); err != nil {
				return err
			}
		}
		_, err := tx.Pipelined(ctx, func(pipe redis.Pipeliner) error {
			if err := pipe.JSONSet(ctx, TxoKey(txo.Outpoint), "$", txo).Err(); err != nil {
				return err
			}
			for _, owner := range txo.Owners {
				if err := pipe.ZAdd(ctx, OwnerTxosKey(owner), redis.Z{
					Score:  score,
					Member: outpoint,
				}).Err(); err != nil {
					return err
				} else if err := pipe.Publish(ctx, PubOwnerKey(owner), outpoint).Err(); err != nil {
					return err
				}
			}
			for acct := range accts {
				if err := pipe.ZAdd(ctx, AccountTxosKey(acct), redis.Z{
					Score:  score,
					Member: outpoint,
				}).Err(); err != nil {
					return err
				} else if err := pipe.Publish(ctx, PubAccountKey(acct), outpoint).Err(); err != nil {
					return err
				} else {
					log.Println("Publish", PubAccountKey(acct), outpoint)
				}
			}
			for tag, data := range txo.Data {
				if data.Validate {
					if err := pipe.ZAdd(ctx, ValidateKey(tag), redis.Z{
						Score:  score,
						Member: outpoint,
					}).Err(); err != nil {
						return err
					}
				}
				for _, event := range data.Events {
					pipe.Publish(ctx, PubEventKey(tag, event), outpoint)
				}
			}
			return nil
		})
		return err
	}, spendKey); err != nil {
		log.Panic(err)
	}
	return nil
}

func (idxCtx *IndexContext) SaveSpends(ctx context.Context) error {
	for vin := range idxCtx.Spends {
		if err := idxCtx.SaveSpend(ctx, uint32(vin)); err != nil {
			return err
		}
	}
	return nil
}

func (idxCtx *IndexContext) SaveSpend(ctx context.Context, vin uint32) error {
	spend := idxCtx.Spends[vin]
	score := float64(idxCtx.Height) + (float64(idxCtx.Idx) / 1000000000)
	// outpoint := spend.Outpoint.String()
	accts := map[string]struct{}{}
	if len(spend.Owners) > 0 {
		if result, err := Rdb.HMGet(ctx, OwnerAccountKey, spend.Owners...).Result(); err != nil {
			log.Panic(err)
			return err
		} else {
			for _, item := range result {
				if acct, ok := item.(string); ok && acct != "" {
					accts[acct] = struct{}{}
				}
			}
		}
	}
	if _, err := Rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		if err := pipe.Set(ctx,
			SpendKey(spend.Outpoint),
			SpendValue(spend.Outpoint.Txid(), score),
			0,
		).Err(); err != nil {
			return err
		}
		txid := idxCtx.Txid.String()
		for _, owner := range spend.Owners {
			ownerKey := OwnerTxosKey(owner)
			if err := pipe.ZAdd(ctx, ownerKey, redis.Z{
				Score:  score,
				Member: txid,
			}).Err(); err != nil {
				return err
				// } else if err := pipe.ZAdd(ctx, ownerKey, redis.Z{
				// 	Score:  -1 * score,
				// 	Member: outpoint,
				// }).Err(); err != nil {
				// 	return err
			} else if err := pipe.Publish(ctx, PubOwnerKey(owner), txid).Err(); err != nil {
				return err
			}
		}
		for acct := range accts {
			if err := pipe.ZAdd(ctx, AccountTxosKey(acct), redis.Z{
				Score:  score,
				Member: txid,
			}).Err(); err != nil {
				return err
				// } else if err := pipe.ZAdd(ctx, AccountTxosKey(acct), redis.Z{
				// 	Score:  -1 * score,
				// 	Member: outpoint,
				// }).Err(); err != nil {
				// 	return err
			} else if err := pipe.Publish(ctx, PubAccountKey(acct), txid).Err(); err != nil {
				return err
			} else {
				log.Println("Publish", PubAccountKey(acct), txid)
			}
		}
		return nil
	}); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}
