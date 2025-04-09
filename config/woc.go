package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bsv-blockchain/go-sdk/transaction"
)

type WOCNetwork string

var (
	Mainnet WOCNetwork = "main"
	Testnet WOCNetwork = "test"
)

type WhatsOnChainBroadcast struct {
	Network WOCNetwork
	ApiKey  string
}

func (b *WhatsOnChainBroadcast) Broadcast(t *transaction.Transaction) (*transaction.BroadcastSuccess, *transaction.BroadcastFailure) {
	ctx := context.Background()
	bodyMap := map[string]interface{}{
		"txhex": t.Hex(),
	}
	if body, err := json.Marshal(bodyMap); err != nil {
		return nil, &transaction.BroadcastFailure{
			Code:        "500",
			Description: err.Error(),
		}
	} else {
		url := fmt.Sprintf("https://api.whatsonchain.com/v1/bsv/%s/tx/raw", b.Network)
		req, err := http.NewRequestWithContext(
			ctx,
			"POST",
			url,
			bytes.NewBuffer(body),
		)
		if err != nil {
			return nil, &transaction.BroadcastFailure{
				Code:        "500",
				Description: err.Error(),
			}
		}
		req.Header.Set("Content-Type", "application/json")
		if b.ApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+b.ApiKey)
		}

		if resp, err := http.DefaultClient.Do(req); err != nil {
			return nil, &transaction.BroadcastFailure{
				Code:        "500",
				Description: err.Error(),
			}
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				if body, err := io.ReadAll(resp.Body); err != nil {
					return nil, &transaction.BroadcastFailure{
						Code:        fmt.Sprintf("%d", resp.StatusCode),
						Description: "unknown error",
					}
				} else {
					return nil, &transaction.BroadcastFailure{
						Code:        fmt.Sprintf("%d", resp.StatusCode),
						Description: string(body),
					}
				}
			} else {
				return &transaction.BroadcastSuccess{
					Txid: t.TxID().String(),
				}, nil
			}
		}
	}
}
