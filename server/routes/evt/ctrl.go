package evt

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:tag/:id/:value", TxosByEvent)
}

func TxosByEvent(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	if txos, err := idx.SearchTxos(c.Context(), &idx.SearchCfg{
		Key: evt.EventKey(c.Params("tag"), &evt.Event{
			Id:    c.Params("id"),
			Value: c.Params("value"),
		}),
		From:          c.QueryFloat("from", 0),
		Reverse:       c.QueryBool("rev", false),
		Limit:         uint32(c.QueryInt("limit", 100)),
		IncludeTxo:    c.QueryBool("txo", false),
		IncludeTags:   tags,
		IncludeScript: c.QueryBool("script", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
