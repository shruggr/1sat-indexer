package own

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:owner/utxos", OwnerUtxos)
	r.Get("/:owner/balance", OwnerBalance)
}

func OwnerUtxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
		Keys:          []string{idx.OwnerTxosKey(owner)},
		From:          &from,
		Reverse:       c.QueryBool("rev", false),
		Limit:         uint32(c.QueryInt("limit", 100)),
		IncludeTxo:    c.QueryBool("txo", false),
		IncludeTags:   tags,
		IncludeScript: c.QueryBool("script", false),
		FilterSpent:   true,
		RefreshSpends: c.QueryBool("refresh", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

func OwnerBalance(c *fiber.Ctx) error {
	owner := c.Params("owner")
	if balance, err := ingest.Store.SearchBalance(c.Context(), &idx.SearchCfg{
		Keys:          []string{idx.OwnerTxosKey(owner)},
		RefreshSpends: c.QueryBool("refresh", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(balance)
	}
}
