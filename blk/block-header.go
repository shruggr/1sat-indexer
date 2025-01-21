package blk

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/bitcoin-sv/go-sdk/chainhash"
)

type BlockHeader struct {
	Hash       *chainhash.Hash `json:"hash"`
	Coin       uint32          `json:"coin"`
	Height     uint32          `json:"height"`
	Time       uint32          `json:"time"`
	Nonce      uint32          `json:"nonce"`
	Version    uint32          `json:"version"`
	MerkleRoot *chainhash.Hash `json:"merkleroot"`
	Bits       string          `json:"bits"`
	Synced     uint64          `json:"synced"`
}

func (b BlockHeader) MarshalBinary() (data []byte, err error) {
	return json.Marshal(b)
}

func (b *BlockHeader) UnmarshalBinary(data []byte) (err error) {
	return json.Unmarshal(data, b)
}

func FetchBlockHeaders(fromBlock uint64, pageSize uint) (blocks []*BlockHeader, err error) {
	url := fmt.Sprintf("%s/v1/block_header/list/%d?limit=%d", JUNGLEBUS, fromBlock, pageSize)
	log.Printf("Requesting %d blocks from height %d\n", pageSize, fromBlock)
	if resp, err := http.Get(url); err != nil {
		log.Panicln("Failed to get blocks from junglebus", err)
	} else if resp.StatusCode != http.StatusOK {
		log.Panicln("Failed to get blocks from junglebus", resp.StatusCode)
	} else {
		err := json.NewDecoder(resp.Body).Decode(&blocks)
		resp.Body.Close()
		if err != nil {
			log.Panic(err)
		}
	}
	return
}
