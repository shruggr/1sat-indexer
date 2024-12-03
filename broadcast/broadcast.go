package broadcast

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/bitcoin-sv/go-sdk/spv"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type BroadcaseResponse struct {
	Success bool   `json:"success"`
	Status  uint32 `json:"status"`
	Txid    string `json:"txid"`
	Error   string `json:"error"`
}

func Broadcast(ctx context.Context, store idx.TxoStore, tx *transaction.Transaction, broadcaster transaction.Broadcaster) (response *BroadcaseResponse) {
	txid := tx.TxID()
	response = &BroadcaseResponse{
		Txid:   txid.String(),
		Status: 500,
	}
	log.Println("Broadcasting", response.Txid, tx.Hex())

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

	score := idx.HeightScore(0, 0)

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
	} else if err = store.Log(ctx, idx.PendingTxLog, response.Txid, -score); err != nil {
		response.Error = err.Error()
		return
	}

	// Check and Mark Spends
	for vin, spendOutpoint := range spendOutpoints {
		// spendOutpoint := spend.Outpoint.String()
		if added, err := store.SetNewSpend(ctx, spendOutpoint, response.Txid); err != nil {
			rollbackSpends(ctx, store, spendOutpoints[:vin], response.Txid)
			response.Error = err.Error()
			return
		} else if !added {
			if spend, err := store.GetSpend(ctx, spendOutpoint); err != nil {
				rollbackSpends(ctx, store, spendOutpoints[:vin], response.Txid)
				response.Error = err.Error()
				return
			} else if spend != response.Txid {
				rollbackSpends(ctx, store, spendOutpoints[:vin], response.Txid)
				response.Status = 409
				response.Error = fmt.Sprintf("double-spend: %s:%d - %s spent in %s", response.Txid, vin, spendOutpoint, spend)
				return
			}
		}
	}

	if success, failure := broadcaster.Broadcast(tx); failure != nil {
		rollbackSpends(ctx, store, spendOutpoints, response.Txid)
		if status, err := strconv.Atoi(failure.Code); err == nil {
			response.Status = uint32(status)
		}
		response.Error = failure.Description
		return
	} else {
		store.Log(ctx, idx.PendingTxLog, response.Txid, score)
		log.Println("Broadcasted", response.Txid, success)
		response.Success = true
		response.Status = 200
	}

	return

}

func rollbackSpends(ctx context.Context, store idx.TxoStore, outpoints []string, txid string) error {
	if len(outpoints) == 0 {
		return nil
	}
	deletes := make([]string, 0, len(outpoints))
	if spends, err := store.GetSpends(ctx, outpoints); err != nil {
		return err
	} else {
		for i, spend := range spends {
			if spend == txid {
				deletes = append(deletes, outpoints[i])
			}
		}
	}
	if len(deletes) > 0 {
		if err := store.UnsetSpends(ctx, deletes); err != nil {
			return err
		}
	}
	if err := store.Delog(ctx, idx.PendingTxLog, txid); err != nil {
		return err
	}
	return nil
}
