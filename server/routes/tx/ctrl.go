package tx

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/b-open-io/go-junglebus"
	"github.com/b-open-io/overlay/beef"
	"github.com/b-open-io/overlay/pubsub"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

// TxController handles indexer-specific transaction routes
type TxController struct {
	IngestCtx   *idx.IngestCtx
	BeefStorage *beef.Storage
	PubSub      pubsub.PubSub
	JungleBus   *junglebus.Client
}

// NewTxController creates a new TxController with the given dependencies
func NewTxController(
	ingestCtx *idx.IngestCtx,
	_ *broadcaster.Arc, // kept for compatibility, not used
	beefStorage *beef.Storage,
	_ interface{}, // chaintracks - kept for compatibility, not used
	ps pubsub.PubSub,
	jb *junglebus.Client,
) *TxController {
	return &TxController{
		IngestCtx:   ingestCtx,
		BeefStorage: beefStorage,
		PubSub:      ps,
		JungleBus:   jb,
	}
}

// RegisterRoutes registers indexer-specific transaction routes
func (ctrl *TxController) RegisterRoutes(r fiber.Router) {
	r.Get("/:txid/txos", ctrl.TxosByTxid)
	r.Get("/:txid/parse", ctrl.ParseTx)
	r.Post("/parse", ctrl.ParseTx)
	r.Post("/:txid/ingest", ctrl.IngestTx)
}

// @Summary ARC transaction callback
// @Description Receive ARC broadcast status callbacks
// @Tags transactions
// @Accept json
// @Param callback body object true "ARC callback response"
// @Success 200 {string} string "OK"
// @Failure 400 {string} string "Invalid callback payload"
// @Router /callback [post]
func (ctrl *TxController) TxCallback(c *fiber.Ctx) error {
	// Parse the ARC callback response
	var arcResp broadcaster.ArcResponse
	if err := c.BodyParser(&arcResp); err != nil {
		log.Printf("[ARC] Error parsing ARC callback: %v", err)
		return c.Status(400).SendString("Invalid callback payload")
	}

	// Validate required fields
	if arcResp.Txid == "" || arcResp.TxStatus == nil {
		log.Printf("[ARC] Missing required fields in callback: txid=%s, status=%v", arcResp.Txid, arcResp.TxStatus)
		return c.Status(400).SendString("Missing txid or txStatus")
	}

	log.Printf("[ARC] %s Callback received: status=%s", arcResp.Txid, *arcResp.TxStatus)

	// Publish the status update for the broadcast listener
	if jsonData, err := json.Marshal(arcResp); err != nil {
		log.Printf("[ARC] %s Error marshaling ARC response: %v", arcResp.Txid, err)
	} else if err := ctrl.PubSub.Publish(c.Context(), "arc", string(jsonData)); err != nil {
		log.Printf("[ARC] %s Error publishing to arc channel: %v", arcResp.Txid, err)
	}

	return c.SendStatus(200)
}

// @Summary Parse transaction
// @Description Parse a transaction and return indexed data
// @Tags transactions
// @Accept octet-stream
// @Produce json
// @Param txid path string false "Transaction ID (if using GET)"
// @Param transaction body string false "Transaction bytes (if using POST)"
// @Success 200 {object} idx.IndexContext
// @Failure 400 {string} string "Invalid transaction"
// @Failure 404 {string} string "Transaction not found"
// @Failure 500 {string} string "Internal server error"
// @Router /idx/{txid}/parse [get]
// @Router /idx/parse [post]
func (ctrl *TxController) ParseTx(c *fiber.Ctx) (err error) {
	txidStr := c.Params("txid")
	var tx *transaction.Transaction
	var rawtx []byte
	if txidStr != "" {
		hash, err := chainhash.NewHashFromHex(txidStr)
		if err != nil {
			return c.SendStatus(400)
		}
		if tx, err = ctrl.BeefStorage.LoadTx(c.Context(), hash); err != nil {
			if err == beef.ErrNotFound {
				return c.SendStatus(404)
			}
			return err
		}
		if tx == nil {
			return c.SendStatus(404)
		}
	} else if err = c.BodyParser(rawtx); err != nil {
		return
	} else if len(rawtx) == 0 {
		return c.SendStatus(400)
	} else if tx, err = transaction.NewTransactionFromBytes(rawtx); err != nil {
		return
	}
	if idxCtx, err := ctrl.IngestCtx.ParseTx(c.Context(), tx); err != nil {
		return err
	} else {
		return c.JSON(idxCtx)
	}
}

// @Summary Ingest transaction
// @Description Force ingest a transaction by txid
// @Tags transactions
// @Produce json
// @Param txid path string true "Transaction ID"
// @Success 200 {object} idx.IndexContext
// @Failure 404 {string} string "Transaction not found"
// @Failure 500 {string} string "Internal server error"
// @Router /idx/{txid}/ingest [post]
func (ctrl *TxController) IngestTx(c *fiber.Ctx) error {
	hash, err := chainhash.NewHashFromHex(c.Params("txid"))
	if err != nil {
		return c.SendStatus(400)
	}
	tx, err := ctrl.BeefStorage.LoadTx(c.Context(), hash)
	if err != nil {
		if err == beef.ErrNotFound {
			return c.SendStatus(404)
		}
		return err
	}
	if tx == nil {
		return c.SendStatus(404)
	}
	if idxCtx, err := ctrl.IngestCtx.IngestTx(c.Context(), tx); err != nil {
		return err
	} else {
		return c.JSON(idxCtx)
	}
}

// @Summary Get TXOs by transaction ID
// @Description Get all transaction outputs for a specific transaction
// @Tags transactions
// @Produce json
// @Param txid path string true "Transaction ID"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Param script query bool false "Include script data"
// @Param spend query bool false "Include spend information"
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /idx/{txid}/txos [get]
func (ctrl *TxController) TxosByTxid(c *fiber.Ctx) error {
	txid := c.Params("txid")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ctrl.IngestCtx.IndexedTags()
	}
	if txos, err := ctrl.IngestCtx.Store.LoadTxosByTxid(c.Context(), txid, tags, c.QueryBool("spend", false)); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
