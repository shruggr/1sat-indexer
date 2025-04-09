package blk

import (
	"math/big"
	"time"

	"github.com/bsv-blockchain/go-sdk/chainhash"
)

// BlockHeader defines a single block header, used in SPV validations.
type BlockHeader struct {
	Height        uint32         `json:"height"`
	Hash          chainhash.Hash `json:"hash"`
	Version       uint32         `json:"version"`
	MerkleRoot    chainhash.Hash `json:"merkleRoot"`
	Timestamp     time.Time      `json:"creationTimestamp"`
	Bits          uint32         `json:"-"`
	Nonce         uint32         `json:"nonce"`
	ChainWork     *big.Int       `json:"chainWork"`
	CumulatedWork *big.Int       `json:"work"`
	PreviousBlock chainhash.Hash `json:"prevBlockHash"`
}

// BlockHeaderState is an extended version of the BlockHeader
// that has more important informations. Mostly used in http server endpoints.
type BlockHeaderState struct {
	Header    BlockHeader `json:"header"`
	State     string      `json:"state"`
	ChainWork *big.Int    `json:"chainWork" swaggertype:"string"`
	Height    uint32      `json:"height"`
}
