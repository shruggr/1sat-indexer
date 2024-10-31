package origins

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/onesat"
)

func RegisterRoutes(r fiber.Router) {
	r.Post("/ancestors", OriginAncestors)
	r.Post("/history", OriginHistory)
}

func OriginHistory(c *fiber.Ctx) error {
	var outpoints []string
	if err := c.BodyParser(&outpoints); err != nil {
		return c.SendStatus(400)
	} else if len(outpoints) == 0 {
		return c.SendStatus(400)
	}

	history := make([]*idx.Txo, 0, len(outpoints))
	for _, outpoint := range outpoints {
		if txos, err := idx.SearchTxos(c.Context(), &idx.SearchCfg{
			Key: evt.EventKey("origin", &evt.Event{
				Id:    "outpoint",
				Value: outpoint,
			}),
			From: c.QueryFloat("from", 0),
		}); err != nil {
			return err
		} else {
			history = append(history, txos...)
		}
	}
	return c.JSON(history)
}

func OriginAncestors(c *fiber.Ctx) error {
	var outpoints []string
	if err := c.BodyParser(&outpoints); err != nil {
		return c.SendStatus(400)
	} else if len(outpoints) == 0 {
		return c.SendStatus(400)
	}

	origins := make([]string, 0, len(outpoints))
	outpointMap := make(map[string]struct{}, len(outpoints))
	for _, outpoint := range outpoints {
		outpointMap[outpoint] = struct{}{}
		if data, err := idx.TxoDB.HGet(c.Context(), idx.TxoDataKey(outpoint), "origin").Result(); err != nil {
			return err
		} else {
			origin := onesat.Origin{}
			if err := json.Unmarshal([]byte(data), &origin); err != nil {
				return err
			}
			origins = append(origins, origin.Outpoint.String())
		}
	}

	ancestors := make([]*idx.Txo, 0, len(origins))
	for _, outpoint := range origins {
		if txos, err := idx.SearchTxos(c.Context(), &idx.SearchCfg{
			Key: evt.EventKey("origin", &evt.Event{
				Id:    "outpoint",
				Value: outpoint,
			}),
			From: c.QueryFloat("from", 0),
		}); err != nil {
			return err
		} else {
			for _, txo := range txos {
				if _, ok := outpointMap[txo.Outpoint.String()]; !ok {
					ancestors = append(ancestors, txo)
				}
			}
		}
	}
	return c.JSON(ancestors)
}
