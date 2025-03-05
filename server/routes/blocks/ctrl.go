package blocks

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/blk"
)

func RegisterRoutes(r fiber.Router) {
	r.Get("/tip", GetChaintip)
	r.Get("/height/:height", GetBlockByHeight)
	r.Get("/hash/:hash", GetBlockByHash)
	r.Get("/list/:from", ListBlocks)
}

func GetChaintip(c *fiber.Ctx) error {
	if chaintip, err := blk.GetChaintip(context.Background()); err != nil {
		return err
	} else {
		return c.JSON(blk.NewBlockHeaderResponse(chaintip))
	}
}

func GetBlockByHeight(c *fiber.Ctx) error {
	if height, err := strconv.ParseUint(c.Params("height"), 10, 32); err != nil {
		return c.SendStatus(400)
	} else if block, err := blk.BlockByHeight(c.Context(), uint32(height)); err != nil {
		return err
	} else if block == nil {
		return c.SendStatus(404)
	} else if chaintip, err := blk.GetChaintip(c.Context()); err != nil {
		return err
	} else {
		if block.Height < chaintip.Height-5 {
			c.Set("Cache-Control", "public,max-age=31536000,immutable")
		} else {
			c.Set("Cache-Control", "public,max-age=60")
		}
		return c.JSON(blk.NewBlockHeaderResponse(block))
	}
}

func GetBlockByHash(c *fiber.Ctx) error {
	if block, err := blk.BlockByHash(c.Context(), c.Params("hash")); err != nil {
		return err
	} else if block == nil {
		return c.SendStatus(404)
	} else {
		c.Set("Cache-Control", "public,max-age=31536000,immutable")
		return c.JSON(blk.NewBlockHeaderResponse(block))
	}
}

func ListBlocks(c *fiber.Ctx) error {
	if fromHeight, err := strconv.ParseUint(c.Params("from"), 10, 32); err != nil {
		return c.SendStatus(400)
	} else if headers, err := blk.Blocks(c.Context(), uint32(fromHeight), 10000); err != nil {
		return err
	} else {
		responses := make([]*blk.BlockHeaderResponse, len(headers))
		for i, header := range headers {
			responses[i] = blk.NewBlockHeaderResponse(header)
		}
		return c.JSON(responses)
	}
}
