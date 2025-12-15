package own

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/b-open-io/go-junglebus"
	"github.com/b-open-io/overlay/queue"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx
var jb *junglebus.Client

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx, jungleBus *junglebus.Client) {
	ingest = ingestCtx
	jb = jungleBus
	r.Get("/:owner/txos", OwnerTxos)
	r.Get("/:owner/utxos", OwnerUtxos)
	r.Get("/:owner/balance", OwnerBalance)
	r.Get("/:owner/sync", OwnerSync)
	r.Get("/:owner/sync/stream", OwnerSyncStream)
}

// @Summary Get owner TXOs
// @Description Get transaction outputs owned by a specific owner (address/pubkey/script hash)
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /owner/{owner}/txos [get]
func OwnerTxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	if c.QueryBool("refresh", true) {
		if err := idx.SyncOwner(c.Context(), idx.IngestTag, owner, ingest, jb); err != nil {
			return err
		}
	}

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	ownerKey := "own:" + owner
	logs, err := ingest.Store.Search(c.Context(), &queue.SearchCfg{
		Keys:    [][]byte{[]byte(ownerKey)},
		From:    &from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 100)),
	}, true)
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, string(log.Member))
	}

	if outputs, err := ingest.Store.LoadOutputs(c.Context(), outpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(idx.TxosFromIndexedOutputs(outputs))
	}
}

// @Summary Get owner UTXOs
// @Description Get unspent transaction outputs owned by a specific owner
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param from query number false "Starting score for pagination"
// @Param rev query bool false "Reverse order"
// @Param limit query int false "Maximum number of results" default(100)
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /owner/{owner}/utxos [get]
func OwnerUtxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	ownerKey := "own:" + owner
	// TODO: Phase 3.6 - Use EventDataStorage with JoinTypeDifference (own - osp)
	// For now, query own: key and filter by spend status via LoadOutputs
	logs, err := ingest.Store.Search(c.Context(), &queue.SearchCfg{
		Keys:    [][]byte{[]byte(ownerKey)},
		From:    &from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 100)),
	}, false) // includeSpent=false filters out spent outputs
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, string(log.Member))
	}

	// Load outputs
	outputs, err := ingest.Store.LoadOutputs(c.Context(), outpoints, tags, false)
	if err != nil {
		return err
	}

	// Filter nil entries and convert to Txo
	txos := make([]*idx.Txo, 0, len(outputs))
	for _, output := range outputs {
		if output != nil {
			txos = append(txos, idx.TxoFromIndexedOutput(output))
		}
	}

	return c.JSON(txos)
}

// @Summary Get owner balance
// @Description Get the satoshi balance for a specific owner
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Success 200 {object} object "Balance information"
// @Failure 500 {string} string "Internal server error"
// @Router /owner/{owner}/balance [get]
func OwnerBalance(c *fiber.Ctx) error {
	owner := c.Params("owner")
	ownerKey := "own:" + owner
	// Use SearchBalance which already filters spent outputs
	balance, err := ingest.Store.SearchBalance(c.Context(), &queue.SearchCfg{
		Keys: [][]byte{[]byte(ownerKey)},
	})
	if err != nil {
		return err
	}

	return c.JSON(map[string]uint64{"balance": balance})
}

// SyncResponse contains paginated outputs for wallet sync
type SyncResponse struct {
	Outputs   []SyncOutput `json:"outputs"`
	NextScore float64      `json:"nextScore"`
	Done      bool         `json:"done"`
}

// SyncOutput represents an outpoint with its score and optional spend txid
type SyncOutput struct {
	Outpoint  string  `json:"outpoint"`
	Score     float64 `json:"score"`
	SpendTxid string  `json:"spendTxid,omitempty"`
}

// @Summary Sync owner outpoints
// @Description Get paginated outputs for wallet synchronization. Returns a merged stream of outputs and spends ordered by score. Each outpoint may appear twice - once when created and once when spent. SpendTxid is populated for all outputs that have been spent.
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param from query number false "Starting score for pagination"
// @Param limit query int false "Maximum number of results" default(100)
// @Success 200 {object} SyncResponse
// @Failure 500 {string} string "Internal server error"
// @Router /owner/{owner}/sync [get]
func OwnerSync(c *fiber.Ctx) error {
	owner := c.Params("owner")
	from := c.QueryFloat("from", 0)
	limit := uint32(c.QueryInt("limit", 100))
	log.Printf("[OwnerSync] owner=%s from=%.0f limit=%d", owner, from, limit)

	// Query limit+1 to detect if there are more results
	queryLimit := limit + 1

	ownerKey := "own:" + owner
	ownerSpentKey := ownerKey + ":spnd"
	// Query both outputs and spends, merged by score
	logs, err := ingest.Store.Search(c.Context(), &queue.SearchCfg{
		Keys:  [][]byte{[]byte(ownerKey), []byte(ownerSpentKey)},
		From:  &from,
		Limit: queryLimit,
	}, true)
	if err != nil {
		return err
	}

	done := len(logs) <= int(limit)
	if !done {
		logs = logs[:limit]
	}

	// Collect unique outpoints for spend lookup
	outpoints := make([]string, len(logs))
	for i, log := range logs {
		outpoints[i] = string(log.Member)
	}

	spendTxids, err := ingest.Store.GetSpends(c.Context(), outpoints)
	if err != nil {
		return err
	}

	outputs := make([]SyncOutput, len(logs))
	for i, log := range logs {
		outputs[i] = SyncOutput{
			Outpoint:  string(log.Member),
			Score:     log.Score,
			SpendTxid: spendTxids[i],
		}
	}

	var nextScore float64
	if len(outputs) > 0 {
		nextScore = outputs[len(outputs)-1].Score
	}

	return c.JSON(SyncResponse{
		Outputs:   outputs,
		NextScore: nextScore,
		Done:      done,
	})
}

// @Summary Stream owner sync via SSE
// @Description Stream paginated outputs for wallet synchronization via Server-Sent Events. Streams all outputs until exhausted, then sends a retry directive.
// @Tags owners
// @Produce text/event-stream
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param from query number false "Starting score for pagination"
// @Success 200 {string} string "SSE stream of SyncOutput events"
// @Router /owner/{owner}/sync/stream [get]
func OwnerSyncStream(c *fiber.Ctx) error {
	owner := c.Params("owner")
	// Check for Last-Event-ID header first (sent by browser on reconnect)
	var from float64
	if lastEventID := c.Get("Last-Event-ID"); lastEventID != "" {
		if parsed, err := strconv.ParseFloat(lastEventID, 64); err == nil {
			from = parsed
		}
	} else {
		from = c.QueryFloat("from", 0)
	}
	const batchSize uint32 = 100

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")
	c.Set("X-Accel-Buffering", "no")
	c.Set("Access-Control-Allow-Origin", "*")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ownerKey := "own:" + owner
		ownerSpentKey := ownerKey + ":spnd"
		currentFrom := from

		for {
			// Query batchSize+1 to detect if there are more results
			queryLimit := batchSize + 1
			logs, err := ingest.Store.Search(c.Context(), &queue.SearchCfg{
				Keys:  [][]byte{[]byte(ownerKey), []byte(ownerSpentKey)},
				From:  &currentFrom,
				Limit: queryLimit,
			}, true)
			if err != nil {
				log.Printf("[OwnerSyncStream] Search error: %v", err)
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				w.Flush()
				return
			}

			hasMore := len(logs) > int(batchSize)
			if hasMore {
				logs = logs[:batchSize]
			}

			if len(logs) == 0 {
				// Trigger sync in background so new items are ready when client returns
				go func() {
					if err := idx.SyncOwner(context.Background(), idx.IngestTag, owner, ingest, jb); err != nil {
						log.Printf("[OwnerSyncStream] SyncOwner error: %v", err)
					}
				}()
				// No more results - tell client to retry in 60 seconds
				fmt.Fprintf(w, "event: done\ndata: {}\nretry: 60000\n\n")
				w.Flush()
				return
			}

			// Collect outpoints for spend lookup
			outpoints := make([]string, len(logs))
			for i, log := range logs {
				outpoints[i] = string(log.Member)
			}

			spendTxids, err := ingest.Store.GetSpends(c.Context(), outpoints)
			if err != nil {
				log.Printf("[OwnerSyncStream] GetSpends error: %v", err)
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				w.Flush()
				return
			}

			// Stream each output as an SSE event
			for i, log := range logs {
				output := SyncOutput{
					Outpoint:  string(log.Member),
					Score:     log.Score,
					SpendTxid: spendTxids[i],
				}

				data, err := json.Marshal(output)
				if err != nil {
					continue
				}

				fmt.Fprintf(w, "data: %s\nid: %.0f\n\n", data, log.Score)
				if err := w.Flush(); err != nil {
					// Client disconnected
					return
				}

				currentFrom = log.Score
			}

			if !hasMore {
				// Trigger sync in background so new items are ready when client returns
				go func() {
					if err := idx.SyncOwner(context.Background(), idx.IngestTag, owner, ingest, jb); err != nil {
						log.Printf("[OwnerSyncStream] SyncOwner error: %v", err)
					}
				}()
				// No more results - tell client to retry in 60 seconds
				fmt.Fprintf(w, "event: done\ndata: {}\nretry: 60000\n\n")
				w.Flush()
				return
			}
		}
	})

	return nil
}
