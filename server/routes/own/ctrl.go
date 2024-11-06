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
}

func OwnerUtxos(c *fiber.Ctx) error {
	address := c.Params("address")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	c.ParamsInt("from", 0)
	if txos, err := idx.SearchTxos(c.Context(), &idx.SearchCfg{
		Key:           idx.OwnerTxosKey(address),
		From:          c.QueryFloat("from", 0),
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
