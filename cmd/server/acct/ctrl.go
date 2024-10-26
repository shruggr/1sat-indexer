package acct

import (
	"flag"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
)

var TAG = "acct"

var CONCURRENCY uint
var indexedTags = make([]string, 0, len(config.Indexers))

var ingest *idx.IngestCtx

func init() {
	flag.UintVar(&CONCURRENCY, "c", 1, "Concurrency")
	// flag.Parse()

	ingest = &idx.IngestCtx{
		Indexers:    config.Indexers,
		Concurrency: CONCURRENCY,
		Network:     config.Network,
	}

	for _, indexer := range config.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
}

func RegisterRoutes(r fiber.Router) {
	r.Put("/:account", RegisterAccount)
	r.Get("/:account", AccountActivity)
	r.Get("/:account/utxos", AccountUtxos)
	r.Get("/:account/:from", AccountActivity)
}

func RegisterAccount(c *fiber.Ctx) error {
	account := c.Params("account")
	var owners []string
	if err := c.BodyParser(&owners); err != nil {
		return c.SendStatus(400)
	} else if len(owners) == 0 {
		return c.SendStatus(400)
	}

	if _, err := idx.UpdateAccount(c.Context(), account, owners); err != nil {
		return err
	} else if err := idx.SyncAcct(c.Context(), TAG, account, ingest); err != nil {
		return err
	}

	return c.SendStatus(204)
}

func AccountUtxos(c *fiber.Ctx) error {
	account := c.Params("account")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = indexedTags
	}
	c.ParamsInt("from", 0)
	if txos, err := idx.SearchTxos(c.Context(), &idx.SearchCfg{
		Key:           idx.AccountTxosKey(account),
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

func AccountActivity(c *fiber.Ctx) (err error) {
	from := c.QueryFloat("from", 0)
	if from == 0 {
		from, _ = strconv.ParseFloat(c.Params("from", "0"), 64)
	}
	if results, err := idx.SearchTxns(c.Context(), &idx.SearchCfg{
		Key:     idx.AccountTxosKey(c.Params("account")),
		From:    from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 0)),
	}); err != nil {
		return err
	} else {
		return c.JSON(results)
	}
}
