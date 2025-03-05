package blk

import (
	"github.com/bitcoin-sv/go-sdk/chainhash"
)

// BlockHeader defines a single block header, used in SPV validations.
type BlockHeader struct {
	Height     uint32         `json:"height"`
	Hash       chainhash.Hash `json:"hash"`
	Version    uint32         `json:"version"`
	MerkleRoot chainhash.Hash `json:"merkleRoot"`
	Timestamp  uint32         `json:"creationTimestamp"`
	Bits       uint32         `json:"difficultyTarget"`
	Nonce      uint32         `json:"nonce"`
	// ChainWork     *big.Int       `json:"chainWork"`
	// CumulatedWork *big.Int       `json:"work"`
	PreviousBlock chainhash.Hash `json:"prevBlockHash"`
}

// BlockHeaderState is an extended version of the BlockHeader
// that has more important informations. Mostly used in http server endpoints.
type BlockHeaderState struct {
	Header BlockHeader `json:"header"`
	State  string      `json:"state"`
	// ChainWork *big.Int    `json:"chainWork" swaggertype:"string"`
	Height uint32 `json:"height"`
}

type BlockHeaderResponse struct {
	Height     uint32         `json:"height"`
	Hash       chainhash.Hash `json:"hash"`
	Version    uint32         `json:"version"`
	MerkleRoot chainhash.Hash `json:"merkleRoot"`
	Timestamp  uint32         `json:"creationTimestamp"`
	Bits       uint32         `json:"bits"`
	Nonce      uint32         `json:"nonce"`
	// ChainWork     *big.Int       `json:"chainWork"`
	// CumulatedWork *big.Int       `json:"work"`
	PreviousBlock chainhash.Hash `json:"prevBlockHash"`
}

func NewBlockHeaderResponse(header *BlockHeader) *BlockHeaderResponse {
	return &BlockHeaderResponse{
		Height:        header.Height,
		Hash:          header.Hash,
		Version:       header.Version,
		MerkleRoot:    header.MerkleRoot,
		Timestamp:     header.Timestamp,
		Bits:          header.Bits,
		Nonce:         header.Nonce,
		PreviousBlock: header.PreviousBlock,
	}
}
