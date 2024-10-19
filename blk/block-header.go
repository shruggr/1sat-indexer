package blk

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/GorillaPool/go-junglebus/models"
)

type BlockHeader models.BlockHeader

func (b BlockHeader) MarshalBinary() (data []byte, err error) {
	return json.Marshal(b)
}

func (b *BlockHeader) UnmarshalBinary(data []byte) (err error) {
	return json.Unmarshal(data, b)
}

func FetchBlockHeaders(fromBlock uint64, pageSize uint) (blocks []*BlockHeader, err error) {
	url := fmt.Sprintf("%s/v1/block_header/list/%d?limit=%d", JUNGLEBUS, fromBlock, pageSize)
	log.Printf("Requesting %d blocks from height %d\n", pageSize, fromBlock)
	if resp, err := http.Get(url); err != nil || resp.StatusCode != 200 {
		log.Panicln("Failed to get blocks from junglebus", resp.StatusCode, err)
	} else {
		err := json.NewDecoder(resp.Body).Decode(&blocks)
		resp.Body.Close()
		if err != nil {
			log.Panic(err)
		}
	}
	return
}
