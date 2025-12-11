package own

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:owner/txos", OwnerTxos)
	r.Get("/:owner/utxos", OwnerTxos)
	r.Get("/:owner/balance", OwnerBalance)
}

// @Summary Get owner TXOs
// @Description Get transaction outputs owned by a specific owner (address/pubkey/script hash)
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param refresh query bool false "Refresh owner data from blockchain"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Param txo query bool false "Include TXO data"
// @Param script query bool false "Include script data"
// @Param spend query bool false "Include spend information"
// @Param unspent query bool false "Filter for unspent outputs only" default(true)
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/txos [get]
// @Router /v5/own/{owner}/utxos [get]
func OwnerTxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	if c.QueryBool("refresh", false) {
		if err := idx.SyncOwner(c.Context(), idx.IngestTag, owner, ingest); err != nil {
			return err
		}
	}
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
		Keys:          []string{idx.OwnerKey(owner)},
		From:          &from,
		Reverse:       c.QueryBool("rev", false),
		Limit:         uint32(c.QueryInt("limit", 100)),
		IncludeTxo:    c.QueryBool("txo", false),
		IncludeTags:   tags,
		IncludeScript: c.QueryBool("script", false),
		IncludeSpend:  c.QueryBool("spend", false),
		FilterSpent:   c.QueryBool("unspent", true),
		RefreshSpends: false, //c.QueryBool("refresh", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

// @Summary Get owner balance
// @Description Get the satoshi balance for a specific owner
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param refresh query bool false "Refresh spend data from blockchain"
// @Success 200 {object} object "Balance information"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/balance [get]
func OwnerBalance(c *fiber.Ctx) error {
	owner := c.Params("owner")
	if balance, err := ingest.Store.SearchBalance(c.Context(), &idx.SearchCfg{
		Keys:          []string{idx.OwnerKey(owner)},
		RefreshSpends: c.QueryBool("refresh", false),
	}); err != nil {
		return err
	} else {
		return c.JSON(balance)
	}
}
