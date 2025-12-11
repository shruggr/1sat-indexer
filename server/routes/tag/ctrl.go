package tag

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:tag", TxosByTag)
}

// @Summary Get TXOs by tag
// @Description Search for transaction outputs by tag
// @Tags tags
// @Produce json
// @Param tag path string true "Tag name"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Param txo query bool false "Include TXO data"
// @Param script query bool false "Include script data"
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /v5/tag/{tag} [get]
func TxosByTag(c *fiber.Ctx) error {

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	from := c.QueryFloat("from", 0)
	if txos, err := config.Store.SearchTxos(c.Context(), &idx.SearchCfg{
		Keys:          []string{evt.TagKey(c.Params("tag"))},
		From:          &from,
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
