package blk

import (
	"context"

	"github.com/bitcoin-sv/go-sdk/chainhash"
)

type HeadersClient struct {
	Ctx context.Context
}

func (c *HeadersClient) IsValidRootForHeight(root *chainhash.Hash, height uint32) bool {
	if header, err := BlockByHeight(c.Ctx, height); err != nil {

		return false
	} else {
		return header.Hash.Equal(*root)
	}
}
