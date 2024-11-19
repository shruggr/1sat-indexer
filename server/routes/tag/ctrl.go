package tag

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	redisstore "github.com/shruggr/1sat-indexer/v5/idx/redis-store"
)

var ingest *idx.IngestCtx
var store *redisstore.RedisStore

func init() {
	store = redisstore.NewRedisTxoStore(os.Getenv("REDISTXO"))
}

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:tag", TxosByTag)
}

func TxosByTag(c *fiber.Ctx) error {

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	if txos, err := store.SearchTxos(c.Context(), &idx.SearchCfg{
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
