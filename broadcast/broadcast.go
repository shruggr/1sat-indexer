package broadcast

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/bitcoin-sv/go-sdk/spv"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/config"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/jb"
	"github.com/shruggr/1sat-indexer/lib"
)

type BroadcaseResponse struct {
	Success bool   `json:"success"`
	Status  uint32 `json:"status"`
	Txid    string `json:"txid"`
	Error   string `json:"error"`
}

func Broadcast(ctx context.Context, tx *transaction.Transaction) (response *BroadcaseResponse) {
	var ingest = &idx.IngestCtx{
		Indexers: config.Indexers,
		Network:  config.Network,
		Tag:      "broadcast",
	}

	txid := tx.TxID()
	response = &BroadcaseResponse{
		Txid:   txid.String(),
		Status: 500,
	}
	log.Println("Broadcasting", response.Txid)

	// Load Inputs
	spendOutpoints := make([]string, 0, len(tx.Inputs))
	for vin, input := range tx.Inputs {
		spendOutpoint := lib.NewOutpointFromHash(input.SourceTXID, input.SourceTxOutIndex)
		spendOutpoints = append(spendOutpoints, spendOutpoint.String())
		if input.SourceTxOutput() == nil {
			if sourceTx, err := jb.LoadTx(ctx, spendOutpoint.TxidHex(), false); err != nil {
				response.Error = err.Error()
				return
			} else if sourceTx == nil {
				response.Status = 404
				response.Error = fmt.Sprintf("missing-input: %s:%d -  %s", response.Txid, vin, spendOutpoint.String())
				return
			} else {
				input.SetSourceTxOutput(sourceTx.Outputs[input.SourceTxOutIndex])
			}
		}
	}

	log.Println("Load Spends", response.Txid)

	score := idx.HeightScore(uint32(time.Now().Unix()), 0)
	// TODO: More useful messages

	// TODO: Verify Fees
	// Verify Transaction locally
	if valid, err := spv.VerifyScripts(tx); err != nil {
		response.Error = err.Error()
		return
	} else if !valid {
		response.Status = 400
		response.Error = fmt.Sprintf("validation-failed: %s", txid)
		return
	} else if err := jb.Cache.Set(ctx, jb.TxKey(response.Txid), tx.Bytes(), 0).Err(); err != nil { //
		response.Error = err.Error()
		return
		// Log Transaction Status as pending
	} else if idx.Log(ctx, idx.TxLogTag, response.Txid, -score); err != nil {
		response.Error = err.Error()
		return
	}

	// Check and Mark Spends
	for vin, spendOutpoint := range spendOutpoints {
		// spendOutpoint := spend.Outpoint.String()
		if added, err := idx.TxoDB.HSetNX(ctx, idx.SpendsKey, spendOutpoint, response.Txid).Result(); err != nil {
			if err := rollbackSpends(ctx, spendOutpoints[:vin]); err == nil {
				idx.Delog(ctx, idx.TxLogTag, response.Txid)
			}
			response.Error = err.Error()
			return
		} else if !added {
			if err := rollbackSpends(ctx, spendOutpoints[:vin]); err == nil {
				idx.Delog(ctx, idx.TxLogTag, response.Txid)
			}
			response.Status = 409
			prevSpend := idx.TxoDB.HGet(ctx, idx.SpendsKey, spendOutpoint).String()
			response.Error = fmt.Sprintf("double-spend: %s:%d - %s spent in %s", response.Txid, vin, spendOutpoint, prevSpend)
			return
		}
	}

	if _, failure := config.Broadcaster.Broadcast(tx); failure != nil {
		rollbackSpends(ctx, spendOutpoints)
		if status, err := strconv.Atoi(failure.Code); err == nil {
			response.Status = uint32(status)
		}
		response.Error = failure.Description
		return
	} else {
		response.Success = true
		response.Status = 200
	}
	if _, err := ingest.IngestTx(ctx, tx, idx.AncestorConfig{Load: true, Parse: true}); err != nil {
		response.Error = err.Error()
		return
	}

	return

}

func rollbackSpends(ctx context.Context, outpoints []string) error {
	if len(outpoints) == 0 {
		return nil
	}
	return idx.TxoDB.HDel(ctx, idx.SpendsKey, outpoints...).Err()
}
