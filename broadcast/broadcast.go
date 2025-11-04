package broadcast

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bsv-blockchain/go-sdk/spv"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/evt"
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
	start := time.Now()
	txid := tx.TxID()
	response = &BroadcastResponse{
		Txid:   txid.String(),
		Status: 500,
	}
	log.Printf("[ARC] %s Broadcasting", txid)

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

	log.Printf("[ARC] %s Load Spends (%.2fms)", response.Txid, time.Since(start).Seconds()*1000)
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
	if valid, err := spv.VerifyScripts(ctx, tx); err != nil {
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
	if Listener != nil {
		statusChan = Listener.RegisterTxid(response.Txid, 45*time.Second)
		defer Listener.UnregisterTxid(response.Txid)
	} else {
		statusChan = make(chan *broadcaster.ArcResponse, 10)
	}

	// Start broadcast in goroutine
	go func() {
		arcResp, err := arcBroadcaster.ArcBroadcast(ctx, tx)
		if err != nil {
			// Local error - send directly to statusChan
			select {
			case statusChan <- &broadcaster.ArcResponse{
				Status:    500,
				ExtraInfo: err.Error(),
			}:
			case <-time.After(1 * time.Second):
			}
			return
		}
		// Success - publish to Redis, listener will route to statusChan
		if jsonData, err := json.Marshal(arcResp); err != nil {
			log.Printf("[ARC] %s Error marshaling ARC response: %v", response.Txid, err)
		} else if err := evt.Publish(ctx, "arc", string(jsonData)); err != nil {
			log.Printf("[ARC] %s Error publishing to Redis channel arc: %v", response.Txid, err)
		}
	}()

	var arcResp *broadcaster.ArcResponse

	// Wait for status updates from either source
statusLoop:
	for {
		select {
		case <-ctx.Done():
			response.Status = 500
			response.Error = "context cancelled"
			return

		case resp, ok := <-statusChan:
			if !ok {
				// Channel closed (timeout) - query ARC status directly as fallback
				log.Printf("[ARC] %s Timeout waiting for accepted status, querying ARC directly (%.2fms)", txid, time.Since(start).Seconds()*1000)

				if arcStatus, err := arcBroadcaster.Status(response.Txid); err != nil {
					log.Printf("[ARC] %s Error querying ARC status: %v (%.2fms)", txid, err, time.Since(start).Seconds()*1000)
				} else if arcStatus.TxStatus != nil {
					log.Printf("[ARC] %s Queried status: %s (%.2fms)", txid, *arcStatus.TxStatus, time.Since(start).Seconds()*1000)
					arcResp = arcStatus
				}
				break statusLoop
			}

			arcResp = resp

			// Handle error response from broadcast (transaction never sent)
			if arcResp.Status != 0 && arcResp.Status != 200 {
				rollbackSpends(ctx, store, spendOutpoints, response.Txid)
				response.Status = uint32(arcResp.Status)
				response.Error = arcResp.ExtraInfo
				return
			}

			// Check if this is the initial Arc response (has Txid set)
			if arcResp.Txid != "" {
				if arcResp.TxStatus != nil {
					log.Printf("[ARC] %s initial response: %s (%.2fms)", txid, *arcResp.TxStatus, time.Since(start).Seconds()*1000)
				}
			} else if arcResp.TxStatus != nil {
				log.Printf("[ARC] %s Received callback status: %s (%.2fms)", txid, *arcResp.TxStatus, time.Since(start).Seconds()*1000)
			}

			// Check if we got accepted or error status - break and handle
			if arcResp.TxStatus != nil {
				if IsAcceptedStatus(*arcResp.TxStatus) || IsErrorStatus(*arcResp.TxStatus) {
					break statusLoop
				} else {
					log.Printf("[ARC] %s Intermediate status %s, waiting for accepted status (%.2fms)", txid, *arcResp.TxStatus, time.Since(start).Seconds()*1000)
				}
			}
		}
	}

	// Handle final response
	if arcResp != nil && arcResp.TxStatus != nil {
		if IsErrorStatus(*arcResp.TxStatus) {
			rollbackSpends(ctx, store, spendOutpoints, response.Txid)
			response.Status = 400
			response.Error = fmt.Sprintf("Transaction rejected: %s", *arcResp.TxStatus)
			if arcResp.ExtraInfo != "" {
				response.Error += " - " + arcResp.ExtraInfo
			}
			return
		}
	}

	// Success
	store.Log(ctx, idx.PendingTxLog, response.Txid, score)
	if arcResp != nil && arcResp.TxStatus != nil {
		log.Printf("[ARC] %s Broadcasted with status: %s (%.2fms)", txid, *arcResp.TxStatus, time.Since(start).Seconds()*1000)
	} else {
		log.Printf("[ARC] %s Broadcasted (status unknown after timeout) (%.2fms)", txid, time.Since(start).Seconds()*1000)
	}
	response.Success = true
	response.Status = 200
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
