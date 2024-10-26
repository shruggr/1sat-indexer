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

	var tags []string
	if dataTags, ok := c.Queries()["tags"]; !ok {
		tags = indexedTags
	} else {
		tags = strings.Split(dataTags, ",")
	}

	if outpoints, err := idx.SearchUtxos(c.Context(), &idx.SearchCfg{
		Key: idx.OwnerTxosKey(address),
	}); err != nil {
		return err
	} else if txos, err := idx.LoadTxos(c.Context(), outpoints, tags); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
