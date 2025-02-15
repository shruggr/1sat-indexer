package jb

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type AddressTxn struct {
	Address string `json:"address"`
	Txid    string `json:"transaction_id"`
	Height  uint32 `json:"block_height"`
	BlockId string `json:"block_hash"`
	Idx     uint64 `json:"block_index"`
}

func FetchOwnerTxns(address string, lastHeight int) (txns []*AddressTxn, err error) {
	if address == "" {
		return
	}
	url := fmt.Sprintf("%s/v1/address/get/%s/%d", JUNGLEBUS, address, lastHeight)
	if resp, err := http.Get(url); err != nil {
		log.Panic(err)
	} else if resp.StatusCode == 200 {
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&txns); err != nil {
			log.Panic(err)
		}
	} else if resp.StatusCode == 400 {
		return nil, ErrBadRequest
	} else if resp.StatusCode == 404 {
		return nil, ErrNotFound
	} else {
		log.Panic("Bad status ", resp.StatusCode, " from ", url)
	}
	return
}
