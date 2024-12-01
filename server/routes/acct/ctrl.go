package acct

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var TAG string

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Put("/:account", RegisterAccount)
	r.Get("/:account", AccountActivity)
	r.Get("/:account/utxos", AccountUtxos)
	r.Get("/:account/:from", AccountActivity)
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
	} else if err := ingest.Store.SyncAcct(c.Context(), TAG, account, ingest); err != nil {
		return err
	}

	return c.SendStatus(204)
}

func AccountUtxos(c *fiber.Ctx) error {
	account := c.Params("account")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
		Key:           idx.AccountTxosKey(account),
		From:          &from,
		Reverse:       c.QueryBool("rev", false),
		Limit:         uint32(c.QueryInt("limit", 100)),
		IncludeTxo:    c.QueryBool("txo", false),
		IncludeTags:   tags,
		IncludeScript: c.QueryBool("script", false),
		FilterSpent:   true,
	}); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

func AccountActivity(c *fiber.Ctx) (err error) {
	from := c.QueryFloat("from", 0)
	if from == 0 {
		from, _ = strconv.ParseFloat(c.Params("from", "0"), 64)
	}
	if results, err := ingest.Store.SearchTxns(c.Context(), &idx.SearchCfg{
		Key:     idx.AccountTxosKey(c.Params("account")),
		From:    &from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 0)),
	}); err != nil {
		return err
	} else {
		return c.JSON(results)
	}
}