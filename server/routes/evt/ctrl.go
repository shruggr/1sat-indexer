package evt

import (
	"net/url"
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

	decodedValue, _ := url.QueryUnescape(c.Params("value"))
	from := c.QueryFloat("from", 0)
	if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
		Key: evt.EventKey(c.Params("tag"), &evt.Event{
			Id:    c.Params("id"),
			Value: decodedValue,
		}),
		From:          &from,
		Reverse:       c.QueryBool("rev", false),
		Limit:         uint32(c.QueryInt("limit", 100)),
		IncludeTxo:    c.QueryBool("txo", false),
		IncludeTags:   tags,
		IncludeScript: c.QueryBool("script", false),
		FilterSpent:   c.QueryBool("unspent", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
