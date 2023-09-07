package lib

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"

	"github.com/GorillaPool/go-junglebus"
	"github.com/GorillaPool/go-junglebus/models"
	_ "github.com/lib/pq"
	"github.com/libsv/go-bt/v2"
	"github.com/ordishs/go-bitcoin"
	"github.com/redis/go-redis/v9"
)

var TRIGGER = uint32(783968)

// var TxCache *lru.Cache[string, *bt.Tx]

var db *sql.DB
var Rdb *redis.Client
var JBClient *junglebus.Client
var bit *bitcoin.Bitcoind

var GetInput *sql.Stmt
var GetMaxInscriptionNum *sql.Stmt
var GetUnnumbered *sql.Stmt
var InsTxo *sql.Stmt
var InsBareSpend *sql.Stmt
var InsSpend *sql.Stmt
var InsInscription *sql.Stmt
var InsMetadata *sql.Stmt
var InsListing *sql.Stmt
var SetSpend *sql.Stmt
var SetInscriptionId *sql.Stmt
var InsClaim *sql.Stmt

// var SetTxn *sql.Stmt

func Initialize(postgres *sql.DB, rdb *redis.Client) (err error) {
	// db = sdb
	db = postgres
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

	port, _ := strconv.ParseInt(os.Getenv("BITCOIN_PORT"), 10, 32)
	bit, err = bitcoin.New(os.Getenv("BITCOIN_HOST"), int(port), os.Getenv("BITCOIN_USER"), os.Getenv("BITCOIN_PASS"), false)
	if err != nil {
		log.Panic(err)
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

	GetMaxInscriptionNum, err = db.Prepare(`SELECT MAX(num) FROM inscriptions`)
	if err != nil {
		log.Fatal(err)
	}

	GetUnnumbered, err = db.Prepare(`
		SELECT txid, vout 
		FROM inscriptions
		WHERE num = -1 AND height <= $1 AND height > 0
		ORDER BY height, idx, vout`,
	)
	if err != nil {
		log.Fatal(err)
	}

	InsTxo, err = db.Prepare(`INSERT INTO txos(txid, vout, satoshis, acc_sats, lock, origin, height, idx, listing, bsv20)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT(txid, vout) DO UPDATE SET 
			satoshis=EXCLUDED.satoshis,
			acc_sats=EXCLUDED.acc_sats,
			lock=EXCLUDED.lock,
			origin=EXCLUDED.origin,
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			listing=EXCLUDED.listing,
			bsv20=EXCLUDED.bsv20
	`)
	if err != nil {
		log.Fatal(err)
	}

	InsBareSpend, err = db.Prepare(`INSERT INTO txos(txid, vout, satoshis, acc_sats, spend, vin)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT(txid, vout) DO NOTHING
	`)
	if err != nil {
		log.Fatal(err)
	}

	InsSpend, err = db.Prepare(`INSERT INTO txos(txid, vout, satoshis, acc_sats, lock, origin, height, idx, spend)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(txid, vout) DO UPDATE SET 
			satoshis=EXCLUDED.satoshis,
			acc_sats=EXCLUDED.acc_sats,
			lock=EXCLUDED.lock,
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
			SET height=EXCLUDED.height, idx=EXCLUDED.idx, origin=EXCLUDED.origin, map=EXCLUDED.map, sigma=EXCLUDED.sigma
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
		INSERT INTO ordinal_lock_listings(txid, vout, height, idx, price, payout, origin, num, spend, lock, bsv20)
		SELECT $1, $2, $3, $4, $5, $6, $7, i.num, t.spend, t.lock, t.bsv20
		FROM txos t
		JOIN inscriptions i ON i.origin = t.origin
		WHERE t.txid=$1 AND t.vout=$2
		ON CONFLICT(txid, vout) DO UPDATE
			SET height=EXCLUDED.height, 
				idx=EXCLUDED.idx, 
				origin=EXCLUDED.origin`,
	)
	if err != nil {
		log.Fatal(err)
	}

	SetInscriptionId, err = db.Prepare(`UPDATE inscriptions
		SET num=$3
		WHERE txid=$1 AND vout=$2
	`)
	if err != nil {
		log.Fatal(err)
	}

	SetSpend, err = db.Prepare(`UPDATE txos
		SET spend=$3, vin=$4
		WHERE txid=$1 AND vout=$2
		RETURNING COALESCE(lock, '\x'), satoshis, listing, bsv20, origin
	`)
	if err != nil {
		log.Fatal(err)
	}

	InsClaim, err = db.Prepare(`INSERT INTO claims(txid, vout, height, idx, origin, sub, type, value)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT(txid, vout, sub, type) DO UPDATE
			SET height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				origin=EXCLUDED.origin,
				value=EXCLUDED.value`,
	)
	if err != nil {
		log.Fatal(err)
	}

	// SetTxn, err = db.Prepare(`INSERT INTO txns(txid, blockid, height, idx)
	// 	VALUES(decode($1, 'hex'), decode($2, 'hex'), $3, $4)
	// 	ON CONFLICT(txid) DO UPDATE SET
	// 		blockid=EXCLUDED.blockid,
	// 		height=EXCLUDED.height,
	// 		idx=EXCLUDED.idx`,
	// )
	// if err != nil {
	// 	log.Fatal(err)
	// }

	return
}

func LoadTx(txid string) (tx *bt.Tx, err error) {
	rawtx, _ := Rdb.Get(context.Background(), txid).Bytes()

	// if len(rawtx) == 0 {
	// 	txData, err := LoadTxData(txid)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	rawtx = txData.Transaction
	// 	Rdb.Set(context.Background(), txid, rawtx, 0).Err()
	// }
	// return bt.NewTxFromBytes(rawtx)

	if len(rawtx) > 0 {
		return bt.NewTxFromBytes(rawtx)
	}

	tx = bt.NewTx()
	r, err := bit.GetRawTransactionRest(txid)
	if err != nil {
		return nil, err
	}
	_, err = tx.ReadFrom(r)
	if err != nil {
		return nil, err
	}
	Rdb.Set(context.Background(), txid, tx.Bytes(), 0).Err()
	return
}

func LoadTxData(txid string) (*models.Transaction, error) {
	// fmt.Printf("Fetching Tx: %s\n", txid)
	txData, err := JBClient.GetTransaction(context.Background(), txid)
	if err != nil {
		return nil, err
	}
	// TxCache.Add(key, txData)
	return txData, nil
}
