package acct

import (
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Put("/:account", RegisterAccount)
	r.Get("/:account", AccountActivity)
	r.Get("/:account/txos", AccountTxos)
	r.Get("/:account/utxos", AccountTxos)
	r.Get("/:account/balance", AccountBalance)
	r.Get("/:account/:from", AccountActivity)
	// r.Put("/:account/tx", RegisterAccount)
}

// @Summary Register account
// @Description Register or update an account with associated owner identifiers
// @Tags accounts
// @Accept json
// @Param account path string true "Account name"
// @Param owners body []string true "Array of owner identifiers"
// @Success 204 "Account registered successfully"
// @Failure 400 {string} string "Invalid request"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/acct/{account} [put]
func RegisterAccount(c *fiber.Ctx) error {
	account := c.Params("account")
	var owners []string
	if err := c.BodyParser(&owners); err != nil {
		return c.SendStatus(400)
	} else if len(owners) == 0 {
		return c.SendStatus(400)
	}

	log.Printf("RegisterAccount: registering account %s with %d owners", account, len(owners))
	if err := ingest.Store.UpdateAccount(c.Context(), account, owners); err != nil {
		log.Printf("RegisterAccount: error updating account %s: %v", account, err)
		return err
	}

	log.Printf("RegisterAccount: starting sync for account %s", account)
	if err := idx.SyncAcct(c.Context(), idx.IngestTag, account, ingest); err != nil {
		log.Printf("RegisterAccount: error syncing account %s: %v", account, err)
		return err
	}

	log.Printf("RegisterAccount: successfully registered account %s", account)
	return c.SendStatus(204)
}

// @Summary Get account TXOs
// @Description Get transaction outputs for an account
// @Tags accounts
// @Produce json
// @Param account path string true "Account name"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Param txo query bool false "Include TXO data"
// @Param script query bool false "Include script data"
// @Param spend query bool false "Include spend information"
// @Param unspent query bool false "Filter for unspent outputs only" default(true)
// @Param refresh query bool false "Refresh spend data from blockchain"
// @Success 200 {array} idx.Txo
// @Failure 404 {string} string "Account not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/acct/{account}/txos [get]
// @Router /v5/acct/{account}/utxos [get]
func AccountTxos(c *fiber.Ctx) error {
	account := c.Params("account")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	limit := uint32(c.QueryInt("limit", 100))
	if owners, err := ingest.Store.AcctOwners(c.Context(), account); err != nil {
		return err
	} else if len(owners) == 0 {
		return c.SendStatus(404)
	} else {
		keys := make([]string, 0, len(owners))
		for _, owner := range owners {
			keys = append(keys, idx.OwnerKey(owner))
		}
		if txos, err := ingest.Store.SearchTxos(c.Context(), &idx.SearchCfg{
			Keys:          keys,
			From:          &from,
			Reverse:       c.QueryBool("rev", false),
			Limit:         limit,
			IncludeTxo:    c.QueryBool("txo", false),
			IncludeTags:   tags,
			IncludeScript: c.QueryBool("script", false),
			IncludeSpend:  c.QueryBool("spend", false),
			FilterSpent:   c.QueryBool("unspent", true),
			RefreshSpends: c.QueryBool("refresh", false),
		}); err != nil {
			return err
		} else {
			return c.JSON(txos)
		}
	}
}

// @Summary Get account balance
// @Description Get the satoshi balance for an account
// @Tags accounts
// @Produce json
// @Param account path string true "Account name"
// @Param refresh query bool false "Refresh spend data from blockchain"
// @Success 200 {object} object "Balance information"
// @Failure 404 {string} string "Account not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/acct/{account}/balance [get]
func AccountBalance(c *fiber.Ctx) error {
	account := c.Params("account")
	if owners, err := ingest.Store.AcctOwners(c.Context(), account); err != nil {
		return err
	} else if len(owners) == 0 {
		return c.SendStatus(404)
	} else {
		keys := make([]string, 0, len(owners))
		for _, owner := range owners {
			keys = append(keys, idx.OwnerKey(owner))
		}
		if balance, err := ingest.Store.SearchBalance(c.Context(), &idx.SearchCfg{
			Keys:          keys,
			RefreshSpends: c.QueryBool("refresh", false),
		}); err != nil {
			return err
		} else {
			return c.JSON(balance)
		}
	}
}

// @Summary Get account activity
// @Description Get transaction activity for an account
// @Tags accounts
// @Produce json
// @Param account path string true "Account name"
// @Param from path number false "Starting score for pagination"
// @Param from query number false "Starting score for pagination (query param)"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results"
// @Success 200 {array} object "Transaction activity"
// @Failure 404 {string} string "Account not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/acct/{account} [get]
// @Router /v5/acct/{account}/{from} [get]
func AccountActivity(c *fiber.Ctx) (err error) {
	from := c.QueryFloat("from", 0)
	if from == 0 {
		from, _ = strconv.ParseFloat(c.Params("from", "0"), 64)
	}
	account := c.Params("account")
	if err := idx.SyncAcct(c.Context(), idx.IngestTag, account, ingest); err != nil {
		return err
	} else if owners, err := ingest.Store.AcctOwners(c.Context(), account); err != nil {
		return err
	} else if len(owners) == 0 {
		return c.SendStatus(404)
	} else {
		keys := make([]string, 0, len(owners))
		for _, owner := range owners {
			keys = append(keys, idx.OwnerKey(owner))
		}
		if results, err := ingest.Store.SearchTxns(c.Context(), &idx.SearchCfg{
			Keys:    keys,
			From:    &from,
			Reverse: c.QueryBool("rev", false),
			Limit:   uint32(c.QueryInt("limit", 0)),
		}); err != nil {
			return err
		} else {
			return c.JSON(results)
		}
	}
}
