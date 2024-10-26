package txos

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
)

var indexedTags = make([]string, 0, len(config.Indexers))

func init() {
	for _, indexer := range config.Indexers {
		indexedTags = append(indexedTags, indexer.Tag())
	}
}

func RegisterRoutes(r fiber.Router) {
	r.Get("/:outpoint", GetTxo)
	r.Post("/", GetTxos)
}

func GetTxo(c *fiber.Ctx) error {
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = indexedTags
	}
	if txo, err := idx.LoadTxo(c.Context(), c.Params("outpoint"), tags); err != nil {
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
		tags = indexedTags
	}
	if txos, err := idx.LoadTxos(c.Context(), outpoints, tags); err != nil {
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
