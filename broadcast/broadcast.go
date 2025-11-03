package broadcast

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/bsv-blockchain/go-sdk/spv"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const MIN_SAT_PER_KB = 1.0

type BroadcastResponse struct {
	Success bool   `json:"success"`
	Status  uint32 `json:"status"`
	Txid    string `json:"txid"`
	Error   string `json:"error"`
}

func Broadcast(ctx context.Context, store idx.TxoStore, tx *transaction.Transaction, arcBroadcaster *broadcaster.Arc) (response *BroadcastResponse) {
	txid := tx.TxID()
	response = &BroadcastResponse{
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
	rawtx := tx.Bytes()
	if fees, err := tx.GetFee(); err != nil {
		response.Error = err.Error()
		return
	} else {
		feeRate := float64(fees) / (float64(len(rawtx)) / 1024.0)
		if feeRate < MIN_SAT_PER_KB {
			response.Status = fiber.StatusPaymentRequired
			response.Error = "fee-too-low"
			return
		}
	}
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
			if spend, err := store.GetSpend(ctx, spendOutpoint, false); err != nil {
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

	// Register for status callbacks BEFORE broadcasting
	var statusChan chan *broadcaster.ArcResponse
	var finalStatus string
	if Listener != nil {
		statusChan = Listener.RegisterTxid(response.Txid, 45*time.Second)
		defer Listener.UnregisterTxid(response.Txid)
	}

	// Broadcast the transaction
	if success, failure := arcBroadcaster.Broadcast(tx); failure != nil {
		rollbackSpends(ctx, store, spendOutpoints, response.Txid)
		if status, err := strconv.Atoi(failure.Code); err == nil {
			response.Status = uint32(status)
		}
		response.Error = failure.Description
		return
	} else {
		log.Println("Broadcast initial response", response.Txid, success.Message)

		// Check if we need to wait for final status via callback
		if statusChan != nil {
			// Try to determine if initial response is already final
			// The success.Message might contain status info, but we should wait for callback
			// to ensure we have the final status

			log.Println("Waiting for final status via callback for", response.Txid)

			select {
			case arcResp := <-statusChan:
				// Received callback with status update
				if arcResp.TxStatus != nil {
					log.Printf("Received callback status for %s: %v", response.Txid, *arcResp.TxStatus)
					finalStatus = string(*arcResp.TxStatus)

					if IsFinalStatus(*arcResp.TxStatus) {
						if IsErrorStatus(*arcResp.TxStatus) {
							// Got error status via callback
							rollbackSpends(ctx, store, spendOutpoints, response.Txid)
							response.Status = 400
							response.Error = fmt.Sprintf("Transaction rejected: %s", *arcResp.TxStatus)
							if arcResp.ExtraInfo != "" {
								response.Error += " - " + arcResp.ExtraInfo
							}
							return
						}
						// Success via callback
						log.Printf("Transaction %s accepted: %v", response.Txid, *arcResp.TxStatus)
					}
				}

			case <-time.After(45 * time.Second):
				// Timeout - query ARC status directly as fallback
				log.Printf("Callback timeout for %s, querying ARC status directly", response.Txid)

				if arcStatus, err := arcBroadcaster.Status(response.Txid); err != nil {
					log.Printf("Error querying ARC status for %s: %v", response.Txid, err)
					// Continue anyway - we got initial success
				} else if arcStatus.TxStatus != nil {
					log.Printf("Queried status for %s: %v", response.Txid, *arcStatus.TxStatus)
					finalStatus = string(*arcStatus.TxStatus)

					if IsErrorStatus(*arcStatus.TxStatus) {
						rollbackSpends(ctx, store, spendOutpoints, response.Txid)
						response.Status = 400
						response.Error = fmt.Sprintf("Transaction rejected: %s", *arcStatus.TxStatus)
						if arcStatus.ExtraInfo != "" {
							response.Error += " - " + arcStatus.ExtraInfo
						}
						return
					}
				}
			}
		}

		// Final success
		store.Log(ctx, idx.PendingTxLog, response.Txid, score)
		log.Printf("Broadcasted %s %s", response.Txid, finalStatus)
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
	if spends, err := store.GetSpends(ctx, outpoints, false); err != nil {
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
