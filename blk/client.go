package blk

import (
	"context"

	"github.com/bsv-blockchain/go-sdk/chainhash"
)

type HeadersClient struct {
	Ctx context.Context
}

func (c *HeadersClient) IsValidRootForHeight(root *chainhash.Hash, height uint32) (bool, error) {
	if header, err := BlockByHeight(c.Ctx, height); err != nil {
		return false, err
	} else {
		return header.MerkleRoot.Equal(*root), nil
	}
}
