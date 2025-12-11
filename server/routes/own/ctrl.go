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
	r.Get("/:owner/utxos", OwnerUtxos)
	r.Get("/:owner/balance", OwnerBalance)
}

// @Summary Get owner TXOs
// @Description Get transaction outputs owned by a specific owner (address/pubkey/script hash)
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/txos [get]
func OwnerTxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys:    []string{idx.OwnerKey(owner)},
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

// @Summary Get owner UTXOs
// @Description Get unspent transaction outputs owned by a specific owner
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/utxos [get]
func OwnerUtxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	// TODO: Phase 3.6 - Use EventDataStorage with JoinTypeDifference (own - osp)
	// For now, query own: key and filter by spend status via LoadTxos
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys:    []string{idx.OwnerKey(owner)},
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

	// Load txos with spend info, then filter unspent
	txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, true)
	if err != nil {
		return err
	}

	utxos := make([]*idx.Txo, 0, len(txos))
	for _, txo := range txos {
		if txo != nil && txo.Spend == "" {
			utxos = append(utxos, txo)
		}
	}

	return c.JSON(utxos)
}

// @Summary Get owner balance
// @Description Get the satoshi balance for a specific owner
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Success 200 {object} object "Balance information"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/balance [get]
func OwnerBalance(c *fiber.Ctx) error {
	owner := c.Params("owner")
	// TODO: Phase 3.6 - Use EventDataStorage with JoinTypeDifference (own - osp)
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys: []string{idx.OwnerKey(owner)},
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, log.Member)
	}

	txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, nil, true)
	if err != nil {
		return err
	}

	var balance uint64
	for _, txo := range txos {
		if txo != nil && txo.Spend == "" {
			balance += *txo.Satoshis
		}
	}

	return c.JSON(map[string]uint64{"balance": balance})
}
