package lib

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	lru "github.com/hashicorp/golang-lru/v2"
	_ "github.com/lib/pq"
	"github.com/libsv/go-bt/v2"
	"github.com/redis/go-redis/v9"
)

var TRIGGER = uint32(783968)

var TxCache *lru.Cache[string, *bt.Tx]

var Rdb *redis.Client
var JBClient *junglebus.Client

var GetInput *sql.Stmt
var GetMaxInscriptionId *sql.Stmt
var GetUnnumbered *sql.Stmt
var InsTxo *sql.Stmt
var InsSpend *sql.Stmt
var InsInscription *sql.Stmt
var InsMetadata *sql.Stmt
var InsListing *sql.Stmt
var SetSpend *sql.Stmt
var SetInscriptionId *sql.Stmt
var SetListing *sql.Stmt
var SetTxn *sql.Stmt

func Initialize(db *sql.DB, rdb *redis.Client) (err error) {
	// db = sdb
	Rdb = rdb
	jb := os.Getenv("JUNGLEBUS")
	if jb == "" {
		jb = os.Getenv("JUNGLEBUS")
	}
	JBClient, err = junglebus.New(
		junglebus.WithHTTP(jb),
	)
	if err != nil {
		return
	}

	GetInput, err = db.Prepare(`SELECT txid, vout, satoshis, acc_sats, lock, COALESCE(spend, '\x'::BYTEA), COALESCE(origin, '\x'::BYTEA)
		FROM txos
		WHERE spend=$1 AND acc_sats>=$2 AND satoshis=1
		ORDER BY acc_sats ASC
		LIMIT 1
	`)
	if err != nil {
		log.Fatal(err)
	}

	GetMaxInscriptionId, err = db.Prepare(`SELECT MAX(id) FROM inscriptions`)
	if err != nil {
		log.Fatal(err)
	}

	GetUnnumbered, err = db.Prepare(`
		SELECT txid, vout 
		FROM inscriptions
		WHERE id = -1 AND height <= $1 AND height > 0
		ORDER BY height, idx, vout`,
	)
	if err != nil {
		log.Fatal(err)
	}

	InsTxo, err = db.Prepare(`INSERT INTO txos(txid, vout, satoshis, acc_sats, lock, origin, height, idx)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT(txid, vout) DO UPDATE SET 
			satoshis=EXCLUDED.satoshis,
			origin=EXCLUDED.origin,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			lock=EXCLUDED.lock
	`)
	if err != nil {
		log.Fatal(err)
	}

	InsSpend, err = db.Prepare(`INSERT INTO txos(txid, vout, satoshis, acc_sats, lock, origin, height, idx, spend)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(txid, vout) DO UPDATE SET 
			satoshis=EXCLUDED.satoshis,
			origin=EXCLUDED.origin,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			spend=EXCLUDED.spend
	`)
	if err != nil {
		log.Fatal(err)
	}

	InsInscription, err = db.Prepare(`
		INSERT INTO inscriptions(txid, vout, height, idx, filehash, filesize, filetype, map, origin, lock, sigma)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT(txid, vout) DO UPDATE
			SET height=EXCLUDED.height, idx=EXCLUDED.idx, origin=EXCLUDED.origin, map=EXCLUDED.map
	`)
	if err != nil {
		log.Panic(err)
	}

	InsMetadata, err = db.Prepare(`
		INSERT INTO metadata(txid, vout, height, idx, ord, map, b, origin, sigma)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(txid, vout) DO UPDATE
			SET height=EXCLUDED.height, idx=EXCLUDED.idx, origin=EXCLUDED.origin
	`)
	if err != nil {
		log.Panic(err)
	}

	InsListing, err = db.Prepare(`
		INSERT INTO ordinal_lock_listings(txid, vout, height, idx, price, payout, origin, num, spend)
		SELECT $1, $2, $3, $4, $5, $6, $7, i.id, t.spend
		FROM txos t
		JOIN inscriptions i ON i.origin = t.origin
		WHERE t.txid=$1 AND t.vout=$2
		ON CONFLICT(txid, vout) DO UPDATE
			SET height=EXCLUDED.height, idx=EXCLUDED.idx, origin=EXCLUDED.origin`,
	)
	if err != nil {
		log.Fatal(err)
	}

	SetInscriptionId, err = db.Prepare(`UPDATE inscriptions
		SET id=$3
		WHERE txid=$1 AND vout=$2
	`)
	if err != nil {
		log.Fatal(err)
	}

	SetSpend, err = db.Prepare(`UPDATE txos
		SET spend=$3, vin=$4
		WHERE txid=$1 AND vout=$2
		RETURNING lock, satoshis, listing, origin
	`)
	if err != nil {
		log.Fatal(err)
	}

	SetTxn, err = db.Prepare(`INSERT INTO txns(txid, blockid, height, idx)
		VALUES(decode($1, 'hex'), decode($2, 'hex'), $3, $4)
		ON CONFLICT(txid) DO UPDATE SET
			blockid=EXCLUDED.blockid,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx`,
	)
	if err != nil {
		log.Fatal(err)
	}

	SetListing, err = db.Prepare(`
		UPDATE txos
		SET listing=true
		WHERE txid=$1 AND vout=$2
		RETURNING lock, origin
	`)
	if err != nil {
		log.Fatal(err)
	}

	SetTxn, err = db.Prepare(`INSERT INTO txns(txid, blockid, height, idx)
		VALUES(decode($1, 'hex'), decode($2, 'hex'), $3, $4)
		ON CONFLICT(txid) DO UPDATE SET
			blockid=EXCLUDED.blockid,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx`,
	)
	if err != nil {
		log.Fatal(err)
	}

	TxCache, err = lru.New[string, *bt.Tx](16 * (2 ^ 20))
	return
}

func LoadTx(txid []byte) (tx *bt.Tx, err error) {
	key := base64.StdEncoding.EncodeToString(txid)
	if tx, ok := TxCache.Get(key); ok {
		return tx, nil
	}
	txData, err := LoadTxData(txid)
	if err != nil {
		return
	}
	tx, err = bt.NewTxFromBytes(txData.Transaction)
	if err != nil {
		return
	}
	TxCache.Add(key, tx)
	return
}

func LoadTxData(txid []byte) (*models.Transaction, error) {

	fmt.Printf("Fetching Tx: %x\n", txid)
	txData, err := JBClient.GetTransaction(context.Background(), hex.EncodeToString(txid))
	if err != nil {
		return nil, err
	}
	// TxCache.Add(key, txData)
	return txData, nil
}

type Outpoint []byte

func NewOutpointFromString(s string) (o *Outpoint, err error) {
	txid, err := hex.DecodeString(s[:64])
	if err != nil {
		return
	}
	vout, err := strconv.ParseUint(s[65:], 10, 32)
	if err != nil {
		return
	}
	origin := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
	o = &origin
	return
}

func (o *Outpoint) String() string {
	return fmt.Sprintf("%x_%d", (*o)[:32], binary.BigEndian.Uint32((*o)[32:]))
}

func (o *Outpoint) Txid() []byte {
	return (*o)[:32]
}

func (o *Outpoint) Vout() uint32 {
	return binary.BigEndian.Uint32((*o)[32:])
}

func (o Outpoint) MarshalJSON() (bytes []byte, err error) {
	if len(o) == 36 {
		bytes, err = json.Marshal(fmt.Sprintf("%x_%d", o[:32], binary.BigEndian.Uint32(o[32:])))
	}
	return bytes, err
}

// UnmarshalJSON deserializes Origin to string
func (o *Outpoint) UnmarshalJSON(data []byte) error {
	var x string
	err := json.Unmarshal(data, &x)
	if err == nil {
		txid, err := hex.DecodeString(x[:64])
		if err != nil {
			return err
		}
		vout, err := strconv.ParseUint(x[65:], 10, 32)
		if err != nil {
			return err
		}

		*o = Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
	}

	return err
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
