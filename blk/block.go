package blk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

var BLOCK_API string
var BLOCK_AUTH_KEY string

var Chaintip *BlockHeader
var C chan *BlockHeader
var updated time.Time

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	BLOCK_API = os.Getenv("BLOCK_API")
	BLOCK_AUTH_KEY = os.Getenv("BLOCK_AUTH_KEY")
}

func StartChaintipSub(ctx context.Context) {
	if C == nil {
		C = make(chan *BlockHeader, 1000)
	}
	go func() {
		for {
			if _, err := GetChaintip(ctx); err != nil {
				log.Panic(err)
			}
			time.Sleep(time.Second)
		}
	}()
}

func GetChaintip(ctx context.Context) (*BlockHeader, error) {
	if time.Since(updated) < 5*time.Second {
		return Chaintip, nil
	}
	headerState := &BlockHeaderState{}
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/chain/tip/longest", BLOCK_API), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+BLOCK_AUTH_KEY)
	if res, err := client.Do(req); err != nil {
		return nil, err
	} else {
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(headerState); err != nil {
			return nil, err
		}
		header := &headerState.Header
		if C != nil && (Chaintip == nil || header.Hash != Chaintip.Hash) {
			C <- header
		}
		header.Height = headerState.Height
		// header.ChainWork = headerState.ChainWork
		Chaintip = header
		updated = time.Now()
		return header, nil
	}
}

func BlockByHeight(ctx context.Context, height uint32) (*BlockHeader, error) {
	headers := []BlockHeader{}
	client := &http.Client{}
	url := fmt.Sprintf("%s/api/v1/chain/header/byHeight?height=%d", BLOCK_API, height)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+BLOCK_AUTH_KEY)
	if res, err := client.Do(req); err != nil {
		return nil, err
	} else {
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(&headers); err != nil {
			return nil, err
		}
		for _, header := range headers {
			if state, err := GetBlockState(ctx, header.Hash.String()); err != nil {
				return nil, err
			} else if state.State == "LONGEST_CHAIN" {
				header.Height = state.Height
				return &header, nil
			}
		}
		header := &headers[0]
		header.Height = height
		return header, nil
	}
}

func BlockByHash(ctx context.Context, hash string) (*BlockHeader, error) {
	if headerState, err := GetBlockState(ctx, hash); err != nil {
		return nil, err
	} else {
		header := &headerState.Header
		header.Height = headerState.Height
		return header, nil
	}
}

func GetBlockState(ctx context.Context, hash string) (*BlockHeaderState, error) {
	headerState := &BlockHeaderState{}
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/chain/header/state/%s", BLOCK_API, hash), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+BLOCK_AUTH_KEY)
	if res, err := client.Do(req); err != nil {
		return nil, err
	} else {
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(headerState); err != nil {
			return nil, err
		}
	}
	return headerState, nil
}

func Blocks(ctx context.Context, fromBlock uint32, count uint) ([]*BlockHeader, error) {
	headers := make([]*BlockHeader, 0, count)
	client := &http.Client{}
	url := fmt.Sprintf("%s/api/v1/chain/header/byHeight?height=%d&count=%d", BLOCK_API, fromBlock, count)
	byHash := make(map[string]*BlockHeader)
	var results []*BlockHeader
	if req, err := http.NewRequest("GET", url, nil); err != nil {
		return nil, err
	} else {
		req.Header.Set("Authorization", "Bearer "+BLOCK_AUTH_KEY)
		if res, err := client.Do(req); err != nil {
			return nil, err
		} else {
			defer res.Body.Close()
			if err := json.NewDecoder(res.Body).Decode(&headers); err != nil {
				return nil, err
			} else if len(headers) == 0 {
				return headers, nil
			}
			for _, header := range headers {
				byHash[header.Hash.String()] = header
			}
			for i := len(headers) - 1; i >= 0; i-- {
				lastHeader := headers[i]
				if state, err := GetBlockState(ctx, lastHeader.Hash.String()); err != nil {
					return nil, err
				} else if state.State == "LONGEST_CHAIN" {
					lastHeight := state.Height
					results = make([]*BlockHeader, lastHeight-fromBlock+1)
					block := &state.Header
					block.Height = state.Height
					results[block.Height-fromBlock] = block
					for {
						parent := block
						if block = byHash[parent.PreviousBlock.String()]; block != nil {
							block.Height = parent.Height - 1
							results[block.Height-fromBlock] = block
						} else {
							return results, nil
						}
					}
				}
			}

			return results, nil
		}
	}
}
