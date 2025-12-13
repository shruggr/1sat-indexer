package blk

import (
	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/chainhash"
)

type BlockHeaderResponse struct {
	Height        uint32         `json:"height"`
	Hash          chainhash.Hash `json:"hash" swaggertype:"string"`
	Version       uint32         `json:"version"`
	MerkleRoot    chainhash.Hash `json:"merkleRoot" swaggertype:"string"`
	Timestamp     uint32         `json:"creationTimestamp"`
	Bits          uint32         `json:"bits"`
	Nonce         uint32         `json:"nonce"`
	PreviousBlock chainhash.Hash `json:"prevBlockHash" swaggertype:"string"`
}

func NewBlockHeaderResponse(header *chaintracks.BlockHeader) *BlockHeaderResponse {
	return &BlockHeaderResponse{
		Height:        header.Height,
		Hash:          header.Hash,
		Version:       uint32(header.Version),
		MerkleRoot:    header.MerkleRoot,
		Timestamp:     header.Timestamp,
		Bits:          header.Bits,
		Nonce:         header.Nonce,
		PreviousBlock: header.PrevHash,
	}
}
