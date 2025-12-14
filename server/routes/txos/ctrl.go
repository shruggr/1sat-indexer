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
	r.Get("/:outpoint/spend", GetSpend)
	r.Post("/", GetTxos)
	r.Post("/spends", GetSpends)
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
// @Router /txo/{outpoint} [get]
func GetTxo(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txo, err := ingest.Store.LoadTxo(c.Context(), c.Params("outpoint"), tags, c.QueryBool("spend", false)); err != nil {
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
// @Router /txo [post]
func GetTxos(c *fiber.Ctx) error {
	var outpoints []string
	if err := c.BodyParser(&outpoints); err != nil {
		return err
	}
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, c.QueryBool("spend", false)); err != nil {
		return err
	} else {
		c.Set("Cache-Control", "public,max-age=60")
		return c.JSON(txos)
	}
}

// @Summary Get spend information
// @Description Get spend information for a transaction output by outpoint
// @Tags txos
// @Produce json
// @Param outpoint path string true "Transaction outpoint (txid_vout)"
// @Success 200 {object} object "Spend information"
// @Failure 500 {string} string "Internal server error"
// @Router /txo/{outpoint}/spend [get]
func GetSpend(c *fiber.Ctx) error {
	outpoint := c.Params("outpoint")
	if spend, err := ingest.Store.GetSpend(c.Context(), outpoint); err != nil {
		return err
	} else {
		return c.JSON(spend)
	}
}

// @Summary Get multiple spend records
// @Description Get spend information for multiple transaction outputs
// @Tags txos
// @Accept json
// @Produce json
// @Param outpoints body []string true "Array of outpoints"
// @Success 200 {array} object "Spend information array"
// @Failure 400 {string} string "Invalid request body"
// @Failure 500 {string} string "Internal server error"
// @Router /txo/spends [post]
func GetSpends(c *fiber.Ctx) error {
	outpoints := []string{}
	if err := c.BodyParser(&outpoints); err != nil {
		return err
	}
	if spends, err := ingest.Store.GetSpends(c.Context(), outpoints); err != nil {
		return err
	} else {
		return c.JSON(spends)
	}
}
