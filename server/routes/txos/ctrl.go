package txos

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:outpoint", GetTxo)
	r.Post("/", GetTxos)
}

// @Summary Get transaction output
// @Description Get a transaction output by outpoint
// @Tags txos
// @Produce json
// @Param outpoint path string true "Transaction outpoint (txid_vout)"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param script query bool false "Include script data"
// @Param spend query bool false "Include spend information"
// @Success 200 {object} idx.Txo
// @Failure 404 {string} string "TXO not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/txo/{outpoint} [get]
func GetTxo(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txo, err := ingest.Store.LoadTxo(c.Context(), c.Params("outpoint"), tags, c.QueryBool("script", false), c.QueryBool("spend", false)); err != nil {
		return err
	} else if txo == nil {
		return c.SendStatus(404)
	} else {
		c.Set("Cache-Control", "public,max-age=60")
		return c.JSON(txo)
	}
}

// @Summary Get multiple transaction outputs
// @Description Get multiple transaction outputs by outpoints
// @Tags txos
// @Accept json
// @Produce json
// @Param outpoints body []string true "Array of outpoints"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param script query bool false "Include script data"
// @Param spend query bool false "Include spend information"
// @Success 200 {array} idx.Txo
// @Failure 400 {string} string "Invalid request body"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/txo [post]
func GetTxos(c *fiber.Ctx) error {
	var outpoints []string
	if err := c.BodyParser(&outpoints); err != nil {
		return err
	}
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, c.QueryBool("script", false), c.QueryBool("spend", false)); err != nil {
		return err
	} else {
		c.Set("Cache-Control", "public,max-age=60")
		return c.JSON(txos)
	}
}
