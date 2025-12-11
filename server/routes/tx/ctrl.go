package tx

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/bsv-blockchain/go-sdk/util"
	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/broadcast"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

var ingest *idx.IngestCtx
var b *broadcaster.Arc

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx, arcBroadcaster *broadcaster.Arc) {
	ingest = ingestCtx
	b = arcBroadcaster
	r.Post("/", BroadcastTx)
	r.Get("/:txid", GetTxWithProof)
	r.Get("/:txid/raw", GetRawTx)
	r.Get("/:txid/proof", GetProof)
	r.Get("/:txid/txos", TxosByTxid)
	r.Get("/:txid/beef", GetTxBEEF)
	r.Post("/callback", TxCallback)
	r.Get("/:txid/parse", ParseTx)
	r.Post("/parse", ParseTx)
	r.Post("/:txid/ingest", IngestTx)
}

// @Summary Broadcast transaction
// @Description Broadcast a transaction to the BSV network
// @Tags transactions
// @Accept octet-stream,plain
// @Produce json
// @Param fmt query string false "Transaction format: 'beef' or standard" Enums(beef)
// @Param transaction body string true "Transaction bytes (binary or hex)"
// @Success 200 {object} broadcast.BroadcastResponse "Broadcast response"
// @Failure 400 {string} string "Invalid transaction"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/tx [post]
func BroadcastTx(c *fiber.Ctx) (err error) {
	var tx *transaction.Transaction
	mime := c.Get("Content-Type")
	switch mime {
	case "application/octet-stream":
		if c.Query("fmt") == "beef" {
			if tx, err = transaction.NewTransactionFromBEEF(c.Body()); err != nil {
				return c.SendStatus(400)
			}
		} else {
			if tx, err = transaction.NewTransactionFromBytes(c.Body()); err != nil {
				return c.SendStatus(400)
			}
		}
	case "text/plain":
		if c.Query("fmt") == "beef" {
			if tx, err = transaction.NewTransactionFromBEEFHex(string(c.Body())); err != nil {
				return c.SendStatus(400)
			}
		} else {
			if tx, err = transaction.NewTransactionFromHex(string(c.Body())); err != nil {
				return c.SendStatus(400)
			}
		}
	default:
		return c.SendStatus(400)
	}

	response := broadcast.Broadcast(c.Context(), ingest.Store, tx, b)
	if response.Success {
		if _, err := ingest.IngestTx(c.Context(), tx, idx.AncestorConfig{Load: true, Parse: true, Save: true}); err != nil {
			log.Println("Ingest Error", tx.TxID().String(), err)
			// } else if out, err := json.MarshalIndent(idxCtx, "", "  "); err != nil {
			// 	log.Println("Ingest Error", tx.TxID().String(), err)
			// } else {
			// 	log.Println("Ingest", tx.TxID().String(), string(out))
		}
		evt.Publish(c.Context(), "broadcast", base64.StdEncoding.EncodeToString(tx.Bytes()))
	} else {
		log.Println("Broadcast Error", tx.TxID().String(), response.Error)
	}
	c.Status(int(response.Status))
	return c.JSON(response)

}

// @Summary Get transaction in BEEF format
// @Description Get a transaction with dependencies in BEEF format
// @Tags transactions
// @Produce octet-stream
// @Param txid path string true "Transaction ID"
// @Success 200 {string} binary "BEEF formatted transaction"
// @Failure 404 {string} string "Transaction not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/tx/{txid}/beef [get]
func GetTxBEEF(c *fiber.Ctx) error {
	if tx, err := jb.BuildTxBEEF(c.Context(), c.Params("txid")); err != nil {
		if err == jb.ErrNotFound {
			return c.SendStatus(404)
		} else {
			return err
		}
	} else if beef, err := tx.AtomicBEEF(true); err != nil {
		return err
	} else {
		c.Set("Content-Type", "application/octet-stream")
		return c.Send(beef)
	}
}

// @Summary Get transaction with proof
// @Description Get a transaction with merkle proof if available
// @Tags transactions
// @Produce octet-stream
// @Param txid path string true "Transaction ID"
// @Success 200 {string} binary "Transaction with proof"
// @Failure 404 {string} string "Transaction not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/tx/{txid} [get]
func GetTxWithProof(c *fiber.Ctx) error {
	txid := c.Params("txid")
	if rawtx, err := jb.LoadRawtx(c.Context(), txid); err != nil {
		if err == jb.ErrNotFound {
			return c.SendStatus(404)
		}
		return err
	} else if len(rawtx) == 0 {
		return c.SendStatus(404)
	} else {
		c.Set("Content-Type", "application/octet-stream")
		buf := bytes.NewBuffer([]byte{})
		buf.Write(util.VarInt(uint64(len(rawtx))).Bytes())
		buf.Write(rawtx)
		if proof, err := jb.LoadProof(c.Context(), txid); err != nil {
			return err
		} else {
			if proof == nil {
				buf.Write(util.VarInt(0).Bytes())
				c.Set("Cache-Control", "public,max-age=60")
			} else {
				if proof.BlockHeight+5 < config.Chaintracks.GetHeight(c.Context()) {
					c.Set("Cache-Control", "public,max-age=31536000,immutable")
				} else {
					c.Set("Cache-Control", "public,max-age=60")
				}
				bin := proof.Bytes()
				buf.Write(util.VarInt(uint64(len(bin))).Bytes())
				buf.Write(bin)
			}
		}
		return c.Send(buf.Bytes())
	}
}

// @Summary Get raw transaction
// @Description Get raw transaction data in binary, hex, or JSON format
// @Tags transactions
// @Produce octet-stream,plain,json
// @Param txid path string true "Transaction ID"
// @Param fmt query string false "Output format: 'bin', 'hex', or 'json'" Enums(bin, hex, json) default(bin)
// @Success 200 {string} binary "Raw transaction"
// @Failure 404 {string} string "Transaction not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/tx/{txid}/raw [get]
func GetRawTx(c *fiber.Ctx) error {
	txid := c.Params("txid")
	if rawtx, err := jb.LoadRawtx(c.Context(), txid); err != nil {
		return err
	} else if rawtx == nil {
		return c.SendStatus(404)
	} else {
		c.Set("Cache-Control", "public,max-age=31536000,immutable")
		switch c.Query("fmt", "bin") {
		case "json":
			if tx, err := transaction.NewTransactionFromBytes(rawtx); err != nil {
				return err
			} else {
				return c.JSON(tx)
			}
		case "hex":
			c.Set("Content-Type", "text/plain")
			return c.SendString(hex.EncodeToString(rawtx))
		default:
			c.Set("Content-Type", "application/octet-stream")
			return c.Send(rawtx)
		}
	}
}

// @Summary Get merkle proof
// @Description Get merkle proof for a transaction
// @Tags transactions
// @Produce octet-stream,plain,json
// @Param txid path string true "Transaction ID"
// @Param fmt query string false "Output format: 'bin', 'hex', or 'json'" Enums(bin, hex, json) default(bin)
// @Success 200 {object} object "Merkle proof"
// @Failure 404 {string} string "Proof not found"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/tx/{txid}/proof [get]
func GetProof(c *fiber.Ctx) error {
	txid := c.Params("txid")
	if proof, err := jb.LoadProof(c.Context(), txid); err != nil {
		return err
	} else if proof == nil {
		return c.SendStatus(404)
	} else {
		if proof.BlockHeight+5 < config.Chaintracks.GetHeight(c.Context()) {
			c.Set("Cache-Control", "public,max-age=31536000,immutable")
		} else {
			c.Set("Cache-Control", "public,max-age=60")
		}
		switch c.Query("fmt", "bin") {
		case "json":
			return c.JSON(proof)
		case "hex":
			c.Set("Content-Type", "text/plain")
			return c.SendString(proof.Hex())
		default:
			c.Set("Content-Type", "application/octet-stream")
			return c.Send(proof.Bytes())
		}
	}
}

// @Summary ARC transaction callback
// @Description Receive ARC broadcast status callbacks
// @Tags transactions
// @Accept json
// @Param callback body object true "ARC callback response"
// @Success 200 {string} string "OK"
// @Failure 400 {string} string "Invalid callback payload"
// @Router /v5/tx/callback [post]
func TxCallback(c *fiber.Ctx) error {
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

	// Publish the status update to Redis for the broadcast listener
	if jsonData, err := json.Marshal(arcResp); err != nil {
		log.Printf("[ARC] %s Error marshaling ARC response: %v", arcResp.Txid, err)
	} else if err := evt.Publish(c.Context(), "arc", string(jsonData)); err != nil {
		log.Printf("[ARC] %s Error publishing to Redis channel arc: %v", arcResp.Txid, err)
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
// @Router /v5/tx/{txid}/parse [get]
// @Router /v5/tx/parse [post]
func ParseTx(c *fiber.Ctx) (err error) {
	txid := c.Params("txid")
	var tx *transaction.Transaction
	var rawtx []byte
	if txid != "" {
		if tx, err = jb.LoadTx(c.Context(), txid, false); err != nil {
			return
		} else if tx == nil {
			return c.SendStatus(404)
		}
	} else if err = c.BodyParser(rawtx); err != nil {
		return
	} else if len(rawtx) == 0 {
		return c.SendStatus(400)
	} else if tx, err = transaction.NewTransactionFromBytes(rawtx); err != nil {
		return
	}
	if idxCtx, err := ingest.ParseTx(c.Context(), tx, idx.AncestorConfig{Load: true, Parse: true, Save: true}); err != nil {
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
// @Router /v5/tx/{txid}/ingest [post]
func IngestTx(c *fiber.Ctx) error {
	txid := c.Params("txid")
	if tx, err := jb.LoadTx(c.Context(), txid, true); err != nil {
		return err
	} else if tx == nil {
		return c.SendStatus(404)
	} else if idxCtx, err := ingest.IngestTx(c.Context(), tx, idx.AncestorConfig{Load: true, Parse: true}); err != nil {
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
// @Router /v5/tx/{txid}/txos [get]
func TxosByTxid(c *fiber.Ctx) error {
	txid := c.Params("txid")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}
	if txos, err := ingest.Store.LoadTxosByTxid(c.Context(), txid, tags, c.QueryBool("script", false), c.QueryBool("spend", false)); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
