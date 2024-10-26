package blocks

import (
	"context"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/blk"
)

var Chaintip *blk.BlockHeader

func init() {
	var err error
	if Chaintip, err = blk.Chaintip(context.Background()); err != nil {
		log.Panic(err)
	}

	go func() {
		if header, c, err := blk.StartChaintipSub(context.Background()); err != nil {
			log.Panic(err)
		} else if header == nil {
			log.Panic("No Chaintip")
		} else {
			log.Println("Chaintip", header.Height, header.Hash)
			Chaintip = header
			for header := range c {
				log.Println("Chaintip", header.Height, header.Hash)
				Chaintip = header
			}
		}
	}()
}

func RegisterRoutes(r fiber.Router) {
	r.Get("/tip", GetChaintip)
	r.Get("/height/:height", GetBlockByHeight)
	r.Get("/hash/:hash", GetBlockByHash)
	r.Get("/list/:from", ListBlocks)
}

func GetChaintip(c *fiber.Ctx) error {
	return c.JSON(Chaintip)
}

func GetBlockByHeight(c *fiber.Ctx) error {
	if height, err := strconv.ParseUint(c.Params("height"), 10, 32); err != nil {
		return c.SendStatus(400)
	} else if block, err := blk.BlockByHeight(c.Context(), uint32(height)); err != nil {
		return err
	} else if block == nil {
		return c.SendStatus(404)
	} else {
		if block.Height < Chaintip.Height-5 {
			c.Set("Cache-Control", "public,max-age=31536000,immutable")
		} else {
			c.Set("Cache-Control", "public,max-age=60")
		}
		return c.JSON(block)
	}
}

func GetBlockByHash(c *fiber.Ctx) error {
	if block, err := blk.BlockByHash(c.Context(), c.Params("hashOrHeight")); err != nil {
		return err
	} else if block == nil {
		return c.SendStatus(404)
	} else {
		c.Set("Cache-Control", "public,max-age=31536000,immutable")
		return c.JSON(block)
	}
}

func ListBlocks(c *fiber.Ctx) error {
	if fromHeight, err := strconv.ParseUint(c.Params("from"), 10, 32); err != nil {
		return c.SendStatus(400)
	} else if headers, err := blk.Blocks(c.Context(), fromHeight, 10000); err != nil {
		return err
	} else {
		return c.JSON(headers)
	}
}
