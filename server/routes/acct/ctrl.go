package acct

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Put("/:account", RegisterAccount)
	r.Get("/:account", AccountActivity)
	r.Get("/:account/txos", AccountTxos)
	r.Get("/:account/utxos", AccountTxos)
	r.Get("/:account/balance", AccountTxos)
	r.Get("/:account/:from", AccountActivity)
	r.Put("/:account/tx", RegisterAccount)
}

func RegisterAccount(c *fiber.Ctx) error {
	account := c.Params("account")
	var owners []string
	if err := c.BodyParser(&owners); err != nil {
		return c.SendStatus(400)
	} else if len(owners) == 0 {
		return c.SendStatus(400)
	}

	if err := ingest.Store.UpdateAccount(c.Context(), account, owners); err != nil {
		return err
	} else if err := SyncAcct(c.Context(), idx.IngestTag, account, ingest); err != nil {
		return err
	}

	return c.SendStatus(204)
}

func AccountTxos(c *fiber.Ctx) error {
	account := c.Params("account")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	limit := uint32(c.QueryInt("limit", 100))
	if owners, err := ingest.Store.AcctOwners(c.Context(), account); err != nil {
		return err
	} else if len(owners) == 0 {
		return c.SendStatus(404)
	} else {
		keys := make([]string, 0, len(owners))
		for _, owner := range owners {
			keys = append(keys, idx.OwnerKey(owner))
		}
		if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
			Keys:          keys,
			From:          &from,
			Reverse:       c.QueryBool("rev", false),
			Limit:         limit,
			IncludeTxo:    c.QueryBool("txo", false),
			IncludeTags:   tags,
			IncludeScript: c.QueryBool("script", false),
			IncludeSpend:  c.QueryBool("spend", false),
			FilterSpent:   c.QueryBool("unspent", true),
			RefreshSpends: c.QueryBool("refresh", false),
		}); err != nil {
			return err
		} else {
			return c.JSON(txos)
		}
	}
}

func AccountBalance(c *fiber.Ctx) error {
	account := c.Params("account")
	if owners, err := ingest.Store.AcctOwners(c.Context(), account); err != nil {
		return err
	} else if len(owners) == 0 {
		return c.SendStatus(404)
	} else {
		keys := make([]string, 0, len(owners))
		for _, owner := range owners {
			keys = append(keys, idx.OwnerKey(owner))
		}
		if balance, err := ingest.Store.SearchBalance(c.Context(), &idx.SearchCfg{
			Keys:          keys,
			RefreshSpends: c.QueryBool("refresh", false),
		}); err != nil {
			return err
		} else {
			return c.JSON(balance)
		}
	}
}

func AccountActivity(c *fiber.Ctx) (err error) {
	from := c.QueryFloat("from", 0)
	if from == 0 {
		from, _ = strconv.ParseFloat(c.Params("from", "0"), 64)
	}
	account := c.Params("account")
	if owners, err := ingest.Store.AcctOwners(c.Context(), account); err != nil {
		return err
	} else if len(owners) == 0 {
		return c.SendStatus(404)
	} else {
		keys := make([]string, 0, len(owners))
		for _, owner := range owners {
			keys = append(keys, idx.OwnerKey(owner))
		}
		if results, err := ingest.Store.SearchTxns(c.Context(), &idx.SearchCfg{
			Keys:    keys,
			From:    &from,
			Reverse: c.QueryBool("rev", false),
			Limit:   uint32(c.QueryInt("limit", 0)),
		}); err != nil {
			return err
		} else {
			return c.JSON(results)
		}
	}
}

func SyncAcct(ctx context.Context, tag string, acct string, ing *idx.IngestCtx) error {
	if owners, err := ingest.Store.AcctOwners(ctx, acct); err != nil {
		return err
	} else {
		for _, own := range owners {
			log.Println("Syncing:", own)
			if lastHeight, err := ingest.Store.LogScore(ctx, idx.OwnerSyncKey, own); err != nil {
				return err
			} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
				log.Println("FetchOwnerTxns:", err)
				return err
			} else {
				limiter := make(chan struct{}, ing.Concurrency)
				var wg sync.WaitGroup
				for _, addTxn := range addTxns {
					if score, err := ingest.Store.LogScore(ctx, tag, addTxn.Txid); err != nil {
						log.Panic(err)
						return err
					} else if score > 0 {
						continue
					}
					wg.Add(1)
					limiter <- struct{}{}
					go func(addTxn *jb.AddressTxn) {
						defer func() {
							<-limiter
							wg.Done()
						}()
						if _, err := ing.IngestTxid(ctx, addTxn.Txid, idx.AncestorConfig{
							Load:  true,
							Parse: true,
							Save:  true,
						}); err != nil {
							log.Panic(err)
						}
					}(addTxn)

					if addTxn.Height > uint32(lastHeight) {
						lastHeight = float64(addTxn.Height)
					}
				}
				wg.Wait()
				if err := ingest.Store.Log(ctx, idx.OwnerSyncKey, own, lastHeight); err != nil {
					log.Panic(err)
				}
			}
		}
	}
	return nil
}

func PutTxns(c *fiber.Ctx) error {
	return nil

}
