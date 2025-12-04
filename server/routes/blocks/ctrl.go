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

// @Summary Get chain tip
// @Description Get the current blockchain tip (highest block)
// @Tags blocks
// @Produce json
// @Success 200 {object} blk.BlockHeaderResponse
// @Failure 500 {string} string "Internal server error"
// @Router /v5/blocks/tip [get]
func GetChaintip(c *fiber.Ctx) error {
	if chaintip, err := blk.GetChaintip(context.Background()); err != nil {
		return err
	} else {
		return c.JSON(blk.NewBlockHeaderResponse(chaintip))
	}
}

// @Summary Get block by height
// @Description Get block header information by block height
// @Tags blocks
// @Produce json
// @Param height path int true "Block height"
// @Success 200 {object} blk.BlockHeaderResponse
// @Failure 400 {string} string "Invalid height"
// @Failure 404 {string} string "Block not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/blocks/height/{height} [get]
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

// @Summary Get block by hash
// @Description Get block header information by block hash
// @Tags blocks
// @Produce json
// @Param hash path string true "Block hash"
// @Success 200 {object} blk.BlockHeaderResponse
// @Failure 404 {string} string "Block not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/blocks/hash/{hash} [get]
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

// @Summary List blocks
// @Description List up to 10,000 block headers starting from a given height
// @Tags blocks
// @Produce json
// @Param from path int true "Starting block height"
// @Success 200 {array} blk.BlockHeaderResponse
// @Failure 400 {string} string "Invalid height"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/blocks/list/{from} [get]
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
