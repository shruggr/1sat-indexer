package tag

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
)

var indexedTags = make([]string, 0, len(config.Indexers))

func init() {
	for _, indexer := range config.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
}

func RegisterRoutes(r fiber.Router) {
	r.Get("/:tag", TxosByTag)
}

func TxosByTag(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = indexedTags
	}

	if txos, err := idx.SearchTxos(c.Context(), &idx.SearchCfg{
		Key:           evt.TagKey(c.Params("tag")),
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
