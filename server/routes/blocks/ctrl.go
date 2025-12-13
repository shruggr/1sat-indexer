package blocks

import (
	"strconv"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/blk"
)

// BlocksController handles block-related routes
type BlocksController struct {
	Chaintracks chaintracks.Chaintracks
}

// NewBlocksController creates a new BlocksController with the given dependencies
func NewBlocksController(ct chaintracks.Chaintracks) *BlocksController {
	return &BlocksController{
		Chaintracks: ct,
	}
}

// RegisterRoutes registers all block routes
func (ctrl *BlocksController) RegisterRoutes(r fiber.Router) {
	r.Get("/tip", ctrl.GetChaintip)
	r.Get("/height/:height", ctrl.GetBlockByHeight)
	r.Get("/hash/:hash", ctrl.GetBlockByHash)
	r.Get("/list/:from", ctrl.ListBlocks)
}

// @Summary Get chain tip
// @Description Get the current blockchain tip (highest block)
// @Tags blocks
// @Produce json
// @Success 200 {object} blk.BlockHeaderResponse
// @Failure 500 {string} string "Internal server error"
// @Router /v5/blocks/tip [get]
func (ctrl *BlocksController) GetChaintip(c *fiber.Ctx) error {
	tip := ctrl.Chaintracks.GetTip(c.Context())
	if tip == nil {
		return c.SendStatus(500)
	}
	return c.JSON(blk.NewBlockHeaderResponse(tip))
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
func (ctrl *BlocksController) GetBlockByHeight(c *fiber.Ctx) error {
	height, err := strconv.ParseUint(c.Params("height"), 10, 32)
	if err != nil {
		return c.SendStatus(400)
	}
	block, err := ctrl.Chaintracks.GetHeaderByHeight(c.Context(), uint32(height))
	if err != nil {
		return err
	}
	if block == nil {
		return c.SendStatus(404)
	}
	if block.Height+5 < ctrl.Chaintracks.GetHeight(c.Context()) {
		c.Set("Cache-Control", "public,max-age=31536000,immutable")
	} else {
		c.Set("Cache-Control", "public,max-age=60")
	}
	return c.JSON(blk.NewBlockHeaderResponse(block))
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
func (ctrl *BlocksController) GetBlockByHash(c *fiber.Ctx) error {
	hash, err := chainhash.NewHashFromHex(c.Params("hash"))
	if err != nil {
		return c.SendStatus(400)
	}
	block, err := ctrl.Chaintracks.GetHeaderByHash(c.Context(), hash)
	if err != nil {
		return err
	}
	if block == nil {
		return c.SendStatus(404)
	}
	c.Set("Cache-Control", "public,max-age=31536000,immutable")
	return c.JSON(blk.NewBlockHeaderResponse(block))
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
func (ctrl *BlocksController) ListBlocks(c *fiber.Ctx) error {
	fromHeight, err := strconv.ParseUint(c.Params("from"), 10, 32)
	if err != nil {
		return c.SendStatus(400)
	}

	tipHeight := ctrl.Chaintracks.GetHeight(c.Context())
	maxCount := uint32(10000)
	if uint32(fromHeight)+maxCount > tipHeight {
		maxCount = tipHeight - uint32(fromHeight) + 1
	}

	responses := make([]*blk.BlockHeaderResponse, 0, maxCount)
	for i := uint32(0); i < maxCount; i++ {
		header, err := ctrl.Chaintracks.GetHeaderByHeight(c.Context(), uint32(fromHeight)+i)
		if err != nil {
			break
		}
		responses = append(responses, blk.NewBlockHeaderResponse(header))
	}
	return c.JSON(responses)
}
