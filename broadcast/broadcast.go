package broadcast

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/bitcoin-sv/go-sdk/spv"
	"github.com/bitcoin-sv/go-sdk/transaction"
	feemodel "github.com/bitcoin-sv/go-sdk/transaction/fee_model"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/jb"
)

var ingest = &idx.IngestCtx{
	Indexers: config.Indexers,
	Tag:      "broadcast",
}

func Broadcast(ctx context.Context, tx *transaction.Transaction) (txidHex string, err error) {
	txid := tx.TxID()
	txidHex = txid.String()
	log.Println("Broadcasting", txidHex)

	for vin, input := range tx.Inputs {
		sourceTxid := input.SourceTXID.String()
		if spend, err := idx.TxoDB.HGet(ctx, idx.SpendsKey, sourceTxid).Result(); err != nil && err != redis.Nil {
			return txidHex, err
		} else if spend != "" {
			log.Printf("double-spend: %s:%d - %s:%d spent in  %s", txidHex, vin, sourceTxid, input.SourceTxOutIndex, spend)
			return txidHex, fmt.Errorf("double-spend: %s:%d - %s:%d spent in  %s", txidHex, vin, sourceTxid, input.SourceTxOutIndex, spend)
		} else if input.SourceTxOutput() == nil {
			if sourceTx, err := jb.LoadTx(ctx, sourceTxid, false); err != nil {
				return txidHex, err
			} else if sourceTx == nil {
				log.Printf("missing-input: %s:%d -  %s:%d - %s\n", txidHex, vin, sourceTxid, input.SourceTxOutIndex, spend)
				return txidHex, fmt.Errorf("missing-input: %s:%d -  %s:%d - %s\n", txidHex, vin, sourceTxid, input.SourceTxOutIndex, spend)
			} else {
				input.SetSourceTxOutput(sourceTx.Outputs[input.SourceTxOutIndex])
			}
		}
	}

	// TODO: More useful messages
	if valid, err := spv.Verify(tx, &spv.GullibleHeadersClient{}, &feemodel.SatoshisPerKilobyte{Satoshis: 1}); err != nil {
		return txidHex, err
	} else if !valid {
		return txidHex, fmt.Errorf("validation-failed: %s", txid)
	}

	rawtx := tx.Bytes()
	score := idx.HeightScore(uint32(time.Now().Unix()), 0)
	idx.Log(ctx, ingest.Tag, txidHex, -score)
	buf := bytes.NewBuffer(rawtx)
	if resp, err := http.Post("https://api.taal.com/api/v1/broadcast", "application/octet-stream", buf); err != nil {
		return txidHex, err
	} else if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Println("Broadcast failed", txid, string(body))
		return txidHex, fmt.Errorf("broadcast-failed: %s", txid)
	}
	jb.Cache.Set(ctx, jb.TxKey(txidHex), rawtx, 0)
	ingest.IngestTx(ctx, tx, idx.AncestorConfig{
		Load:  true,
		Parse: true,
	})
	return txidHex, nil
}
