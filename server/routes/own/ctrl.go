package own

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Get("/:owner/txos", OwnerTxos)
	r.Get("/:owner/utxos", OwnerUtxos)
	r.Get("/:owner/balance", OwnerBalance)
	r.Get("/:owner/sync", OwnerSync)
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
// @Router /v5/own/{owner}/txos [get]
func OwnerTxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys:    []string{idx.OwnerKey(owner)},
		From:    &from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 100)),
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, log.Member)
	}

	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(txos)
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
// @Router /v5/own/{owner}/utxos [get]
func OwnerUtxos(c *fiber.Ctx) error {
	owner := c.Params("owner")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	from := c.QueryFloat("from", 0)
	// TODO: Phase 3.6 - Use EventDataStorage with JoinTypeDifference (own - osp)
	// For now, query own: key and filter by spend status via LoadTxos
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys:    []string{idx.OwnerKey(owner)},
		From:    &from,
		Reverse: c.QueryBool("rev", false),
		Limit:   uint32(c.QueryInt("limit", 100)),
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, log.Member)
	}

	// Load txos with spend info, then filter unspent
	txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, true)
	if err != nil {
		return err
	}

	utxos := make([]*idx.Txo, 0, len(txos))
	for _, txo := range txos {
		if txo != nil && txo.Spend == "" {
			utxos = append(utxos, txo)
		}
	}

	return c.JSON(utxos)
}

// @Summary Get owner balance
// @Description Get the satoshi balance for a specific owner
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Success 200 {object} object "Balance information"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/balance [get]
func OwnerBalance(c *fiber.Ctx) error {
	owner := c.Params("owner")
	// TODO: Phase 3.6 - Use EventDataStorage with JoinTypeDifference (own - osp)
	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys: []string{idx.OwnerKey(owner)},
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, log.Member)
	}

	txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, nil, true)
	if err != nil {
		return err
	}

	var balance uint64
	for _, txo := range txos {
		if txo != nil && txo.Spend == "" {
			balance += *txo.Satoshis
		}
	}

	return c.JSON(map[string]uint64{"balance": balance})
}

// SyncResponse contains paginated unspent and spent outpoints for wallet sync
type SyncResponse struct {
	Unspent   []SyncOutpoint `json:"unspent"`
	Spent     []SyncOutpoint `json:"spent"`
	NextScore float64        `json:"nextScore"`
	Done      bool           `json:"done"`
}

// SyncOutpoint represents an outpoint with its score for sync pagination
type SyncOutpoint struct {
	Outpoint string  `json:"outpoint"`
	Score    float64 `json:"score"`
}

// @Summary Sync owner outpoints
// @Description Get paginated unspent and spent outpoints for wallet synchronization
// @Tags owners
// @Produce json
// @Param owner path string true "Owner identifier (address, pubkey, or script hash)"
// @Param from query number false "Starting score for pagination"
// @Param limit query int false "Maximum number of results per list" default(100)
// @Param spent query bool false "Include already-spent outpoints in unspent list" default(false)
// @Success 200 {object} SyncResponse
// @Failure 500 {string} string "Internal server error"
// @Router /v5/own/{owner}/sync [get]
func OwnerSync(c *fiber.Ctx) error {
	owner := c.Params("owner")
	from := c.QueryFloat("from", 0)
	limit := uint32(c.QueryInt("limit", 100))
	includeSpent := c.QueryBool("spent", false)

	// Query limit+1 to detect if there are more results
	queryLimit := limit + 1

	// Query unspent outpoints (own: with FilterSpent unless spent=true)
	unspentLogs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys:        []string{idx.OwnerKey(owner)},
		From:        &from,
		Limit:       queryLimit,
		FilterSpent: !includeSpent,
	})
	if err != nil {
		return err
	}

	// Query spent outpoints (osp:)
	spentLogs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys:  []string{idx.OwnerSpentKey(owner)},
		From:  &from,
		Limit: queryLimit,
	})
	if err != nil {
		return err
	}

	// Determine if either list hit the limit
	unspentFull := len(unspentLogs) > int(limit)
	spentFull := len(spentLogs) > int(limit)

	// Find the cutoff score - max score from the list that hit limit
	var cutoffScore float64
	if unspentFull && spentFull {
		// Both full - use the lower max score
		unspentMax := unspentLogs[limit].Score
		spentMax := spentLogs[limit].Score
		if unspentMax < spentMax {
			cutoffScore = unspentMax
		} else {
			cutoffScore = spentMax
		}
	} else if unspentFull {
		cutoffScore = unspentLogs[limit].Score
	} else if spentFull {
		cutoffScore = spentLogs[limit].Score
	}

	// Trim lists to cutoff score and convert to response format
	unspent := make([]SyncOutpoint, 0, len(unspentLogs))
	for _, log := range unspentLogs {
		if cutoffScore > 0 && log.Score > cutoffScore {
			break
		}
		unspent = append(unspent, SyncOutpoint{Outpoint: log.Member, Score: log.Score})
	}

	spent := make([]SyncOutpoint, 0, len(spentLogs))
	for _, log := range spentLogs {
		if cutoffScore > 0 && log.Score > cutoffScore {
			break
		}
		spent = append(spent, SyncOutpoint{Outpoint: log.Member, Score: log.Score})
	}

	// Calculate nextScore from the highest score in the trimmed results
	var nextScore float64
	if len(unspent) > 0 && unspent[len(unspent)-1].Score > nextScore {
		nextScore = unspent[len(unspent)-1].Score
	}
	if len(spent) > 0 && spent[len(spent)-1].Score > nextScore {
		nextScore = spent[len(spent)-1].Score
	}

	// Done if neither list was full
	done := !unspentFull && !spentFull

	return c.JSON(SyncResponse{
		Unspent:   unspent,
		Spent:     spent,
		NextScore: nextScore,
		Done:      done,
	})
}
