package own

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:owner/txos", OwnerTxos)
	r.Get("/:owner/utxos", OwnerTxos)
	r.Get("/:owner/balance", OwnerBalance)
}

func OwnerTxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	if c.QueryBool("refresh", false) {
		if err := idx.SyncOwner(c.Context(), idx.IngestTag, owner, ingest); err != nil {
			return err
		}
	}
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
		Keys:          []string{idx.OwnerKey(owner)},
		From:          &from,
		Reverse:       c.QueryBool("rev", false),
		Limit:         uint32(c.QueryInt("limit", 100)),
		IncludeTxo:    c.QueryBool("txo", false),
		IncludeTags:   tags,
		IncludeScript: c.QueryBool("script", false),
		IncludeSpend:  c.QueryBool("spend", false),
		FilterSpent:   c.QueryBool("unspent", true),
		RefreshSpends: false, //c.QueryBool("refresh", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

func OwnerBalance(c *fiber.Ctx) error {
	owner := c.Params("owner")
	if balance, err := ingest.Store.SearchBalance(c.Context(), &idx.SearchCfg{
		Keys:          []string{idx.OwnerKey(owner)},
		RefreshSpends: c.QueryBool("refresh", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(balance)
	}
}
