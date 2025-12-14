package evt

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:tag/:id/:value", TxosByEvent)
}

// @Summary Get TXOs by event
// @Description Search for transaction outputs by event tag, id, and value
// @Tags events
// @Produce json
// @Param tag path string true "Event tag"
// @Param id path string true "Event ID"
// @Param value path string true "Event value"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /evt/{tag}/{id}/{value} [get]
func TxosByEvent(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	decodedValue, _ := url.QueryUnescape(c.Params("value"))
	from := c.QueryFloat("from", 0)
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys: []string{idx.EventKey(c.Params("tag"), &idx.Event{
			Id:    c.Params("id"),
			Value: decodedValue,
		})},
		From:    &from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 100)),
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, log.Member)
	}

	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
