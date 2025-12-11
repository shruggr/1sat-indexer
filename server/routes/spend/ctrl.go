package spend

import (
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:outpoint", GetSpend)
	r.Post("/", GetSpends)
}

// @Summary Get spend information
// @Description Get spend information for a transaction output by outpoint
// @Tags spends
// @Produce json
// @Param outpoint path string true "Transaction outpoint (txid_vout)"
// @Success 200 {object} object "Spend information"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/spends/{outpoint} [get]
func GetSpend(c *fiber.Ctx) error {
	outpoint := c.Params("outpoint")
	if spend, err := ingest.Store.GetSpend(c.Context(), outpoint, true); err != nil {
		return err
	} else {
		return c.JSON(spend)
	}
}

// @Summary Get multiple spend records
// @Description Get spend information for multiple transaction outputs
// @Tags spends
// @Accept json
// @Produce json
// @Param outpoints body []string true "Array of outpoints"
// @Success 200 {array} object "Spend information array"
// @Failure 400 {string} string "Invalid request body"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/spends [post]
func GetSpends(c *fiber.Ctx) error {
	outpoints := []string{}
	if err := c.BodyParser(&outpoints); err != nil {
		return err
	}
	if spends, err := ingest.Store.GetSpends(c.Context(), outpoints, true); err != nil {
		return err
	} else {
		return c.JSON(spends)
	}
}
