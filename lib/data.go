package lib

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	lru "github.com/hashicorp/golang-lru/v2"
	_ "github.com/lib/pq"
	"github.com/libsv/go-bt/v2"
)

var TRIGGER = uint32(783968)

var txCache *lru.ARCCache[string, *models.Transaction]
var Db *sql.DB
var JBClient *junglebus.JungleBusClient
var GetInsNumber *sql.Stmt

func Initialize(db *sql.DB) (err error) {
	Db := db
	jb := os.Getenv("JUNGLEBUS")
	if jb == "" {
		jb = "https://junglebus.gorillapool.io"
	}
	JBClient, err = junglebus.New(
		junglebus.WithHTTP(jb),
	)
	if err != nil {
		return
	}

	GetInsNumber, err = Db.Prepare(`
		SELECT COUNT(i.txid) 
		FROM inscriptions i
		JOIN inscriptions l ON i.height < l.height OR (i.height = l.height AND i.idx < l.idx)
		WHERE l.txid=$1 AND l.vout=$2
	`)
	if err != nil {
		return
	}

	txCache, err = lru.NewARC[string, *models.Transaction](2 ^ 30)
	return
}

func LoadTx(txid []byte) (tx *bt.Tx, err error) {
	txData, err := LoadTxData(txid)
	if err != nil {
		return
	}
	return bt.NewTxFromBytes(txData.Transaction)
}

func LoadTxData(txid []byte) (*models.Transaction, error) {
	key := base64.StdEncoding.EncodeToString(txid)
	if txData, ok := txCache.Get(key); ok {
		return txData, nil
	}
	fmt.Printf("Fetching Tx: %x\n", txid)
	txData, err := JBClient.GetTransaction(context.Background(), hex.EncodeToString(txid))
	if err != nil {
		return nil, err
	}
	txCache.Add(key, txData)
	return txData, nil
}

// ByteString is a byte array that serializes to hex
type ByteString []byte

// MarshalJSON serializes ByteArray to hex
func (s ByteString) MarshalJSON() ([]byte, error) {
	bytes, err := json.Marshal(fmt.Sprintf("%x", string(s)))
	return bytes, err
}

// UnmarshalJSON deserializes ByteArray to hex
func (s *ByteString) UnmarshalJSON(data []byte) error {
	var x string
	err := json.Unmarshal(data, &x)
	if err == nil {
		str, e := hex.DecodeString(x)
		*s = ByteString([]byte(str))
		err = e
	}

	return err
}
