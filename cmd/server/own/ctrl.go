package own

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
)

var indexedTags = make([]string, 0, len(config.Indexers))

func init() {
	for _, indexer := range config.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
}

func RegisterRoutes(r fiber.Router) {
	r.Get("/:owner/utxos", OwnerUtxos)
}

func OwnerUtxos(c *fiber.Ctx) error {
	address := c.Params("address")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = indexedTags
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
