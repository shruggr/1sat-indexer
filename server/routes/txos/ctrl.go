package txos

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	r.Get("/:outpoint", GetTxo)
	r.Post("/", GetTxos)
}

func GetTxo(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txo, err := ingest.Store.LoadTxo(c.Context(), c.Params("outpoint"), tags); err != nil {
		return err
	} else if txo == nil {
		return c.SendStatus(404)
	} else {
		if c.Query("script") == "true" {
			txo.LoadScript(c.Context())
		}
		c.Set("Cache-Control", "public,max-age=60")
		return c.JSON(txo)
	}
}

func GetTxos(c *fiber.Ctx) error {
	var outpoints []string
	if err := c.BodyParser(&outpoints); err != nil {
		return err
	}
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags); err != nil {
		return err
	} else {
		if c.Query("script") == "true" {
			for _, txo := range txos {
				txo.LoadScript(c.Context())
			}
		}
		c.Set("Cache-Control", "public,max-age=60")
		return c.JSON(txos)
	}
}
