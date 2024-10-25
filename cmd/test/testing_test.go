package test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"testing"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/jb"
	"github.com/stretchr/testify/assert"
)

var hexId = "337df7521a168e91dd02ab1a75019d43f69d71274cb830b78568762454f5f27d"
var ctx = context.Background()

func TestMerklePathParseHex(t *testing.T) {
	tx, err := jb.LoadTx(ctx, hexId, true)
	assert.NoError(t, err)
	assert.NotNil(t, tx)
	beef, err := tx.BEEF()
	assert.NoError(t, err)
	log.Println("With Proof:", hex.EncodeToString(beef))

	out, err := json.MarshalIndent(tx, "", "  ")
	log.Println("JSON", string(out))
	_, err = transaction.NewTransactionFromBEEF(beef)
	assert.NoError(t, err)

	tx.MerklePath = nil
	beefNoProof, err := tx.BEEF()
	assert.NoError(t, err)
	log.Println("No Proof:", hex.EncodeToString(beef))

	out, err = json.MarshalIndent(tx, "", "  ")
	log.Println("JSON", string(out))
	_, err = transaction.NewTransactionFromBEEF(beefNoProof)
	assert.NoError(t, err)

}
