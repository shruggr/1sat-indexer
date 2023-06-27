package lib

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/libsv/go-bt/v2"
)

const THREADS = 16

type IndexResult struct {
	Txid       ByteString `json:"txid"`
	Fees       uint64     `json:"fees"`
	Txos       []*Txo     `json:"txos"`
	Spends     []*Txo     `json:"spends"`
	Events     []*Event   `json:"events"`
	Bsv20s     []*Bsv20   `json:"bsv20s"`
	QueryCount uint64
}

type Txn struct {
	Tx       *bt.Tx
	Id       ByteString
	HexId    string
	BlockId  string
	Height   uint32
	Idx      uint64
	Parents  map[string]*Txn
	Children map[string]*Txn
}

func (t *Txn) Bytes() []byte {
	rawtx := t.Tx.Bytes()
	buf := make([]byte, 0, 76+len(rawtx))
	buf = binary.BigEndian.AppendUint32(buf, t.Height)
	buf = binary.BigEndian.AppendUint64(buf, t.Idx)
	buf = append(buf, rawtx...)
	buf = append(buf, []byte(t.BlockId)...)
	return buf
}

func NewTxnFromBytes(b []byte) (txn *Txn, err error) {
	tx, err := bt.NewTxFromBytes(b[12 : len(b)-64])
	if err != nil {
		return
	}
	txid := tx.TxIDBytes()
	txn = &Txn{
		Height:  binary.BigEndian.Uint32(b[:4]),
		Idx:     binary.BigEndian.Uint64(b[4:12]),
		Tx:      tx,
		BlockId: string(b[len(b)-64:]),
		Id:      txid,
		HexId:   hex.EncodeToString(txid),
	}
	return
}

func (t *Txn) Index(dryRun bool) (result *IndexResult, err error) {
	result = &IndexResult{
		Txid: t.Id,
	}
	spendsAcc := map[uint64]*Txo{}
	// t.Id = t.Tx.TxIDBytes()
	var satsIn uint64
	// fmt.Printf("Indexing %s %t \n%x\n", t.HexId, t.Tx.IsCoinbase(), t.Tx.Bytes())
	if !t.Tx.IsCoinbase() {
		for vin, txin := range t.Tx.Inputs {
			spend := &Txo{
				Txid:  txin.PreviousTxID(),
				Vout:  txin.PreviousTxOutIndex,
				Spend: t.Id,
				InAcc: satsIn,
				Vin:   uint32(vin),
			}
			var row *sql.Row
			if dryRun {
				row = Db.QueryRow(`SELECT lock, satoshis, origin, bsv20, listing
					FROM txos
					WHERE txid=$1 AND vout=$2`,
					spend.Txid,
					spend.Vout,
				)
			} else {
				row = Db.QueryRow(`UPDATE txos
					SET spend=$3, inacc=$4, vin=$5
					WHERE txid=$1 AND vout=$2
					RETURNING lock, satoshis, origin, bsv20, listing`,
					spend.Txid,
					spend.Vout,
					spend.Spend,
					spend.InAcc,
					spend.Vin,
				)
			}
			result.QueryCount++

			err := row.Scan(&spend.Lock, &spend.Satoshis, &spend.Origin, &spend.Bsv20, &spend.Listing)
			if err != nil {
				log.Printf("%x %d %s", spend.Txid, spend.Vout, t.HexId)
				log.Panic(err)
			}
			spendsAcc[satsIn] = spend

			satsIn += spend.Satoshis
			outpoint := Outpoint(binary.BigEndian.AppendUint32(spend.Txid, spend.Vout))
			if spend.Listing {
				result.Events = append(result.Events, &Event{
					Channel: "unlist",
					Data:    outpoint[:],
				})
			}
			result.Spends = append(result.Spends, spend)
		}
	}

	var satsOut uint64
	for vout, txout := range t.Tx.Outputs {
		// if txout.Satoshis == 0 {
		// 	continue
		// }

		scripthash := sha256.Sum256(*txout.LockingScript)
		outpoint := Outpoint(binary.BigEndian.AppendUint32(t.Id, uint32(vout)))
		txo := &Txo{
			Txid:       t.Id,
			Vout:       uint32(vout),
			Satoshis:   txout.Satoshis,
			OutAcc:     satsOut,
			Scripthash: scripthash[:],
			Outpoint:   &outpoint,
		}

		if txout.Satoshis < 2 {
			for _, spend := range result.Spends {
				if spend.Satoshis == 1 && txo.Satoshis == 1 && spend.InAcc == txo.OutAcc {
					txo.Origin = spend.Origin
					txo.InOrd = spend
				}
			}
			parsed := ParseScript(*txout.LockingScript, t.Tx, t.Height)
			txo.Parsed = parsed
			txo.Lock = parsed.Lock

			if txo.Origin == nil && parsed.Inscription != nil && txo.Satoshis == 1 && t.Height >= uint32(783968) {
				txo.Origin = txo.Outpoint
			}

			txo.Listing = parsed.Listing != nil
			txo.Bsv20 = parsed.Bsv20 != nil && parsed.Bsv20.Op != "deploy"
			if parsed.Bsv20 != nil {
				result.Bsv20s = append(result.Bsv20s, parsed.Bsv20)
			}

			// result.ParsedScripts = append(result.ParsedScripts, parsed)
		}

		result.Txos = append(result.Txos, txo)
		satsOut += txout.Satoshis
	}

	if !t.Tx.IsCoinbase() {
		result.Fees = satsIn - satsOut
	}

	if !dryRun {
		result.QueryCount++
		_, err = Db.Exec(`INSERT INTO txns(txid, block_id, height, idx, fees)
			VALUES($1, decode($2, 'hex'), $3, $4, $5)
			ON CONFLICT(txid) DO UPDATE SET
				block_id=EXCLUDED.block_id,
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			t.Id,
			t.BlockId,
			t.Height,
			t.Idx,
			result.Fees,
		)
		if err != nil {
			log.Panic(err)
		}
		for _, txo := range result.Txos {
			// Implied BSV20 transfer
			if len(result.Bsv20s) == 0 && txo.InOrd != nil && txo.InOrd.Bsv20 {
				result.QueryCount++
				rows, err := Db.Query(`SELECT tick, amt
					FROM bsv20_txos
					WHERE txid=$1 AND vout=$2 AND op != 'deploy'`,
					txo.InOrd.Txid,
					txo.InOrd.Vout,
				)
				if err != nil {
					log.Panic(err)
				}
				defer rows.Close()
				if rows.Next() {
					bsv20 := &Bsv20{
						Txid:    txo.Txid,
						Vout:    txo.Vout,
						Op:      "transfer",
						Lock:    txo.Lock,
						Implied: true,
					}

					err := rows.Scan(&bsv20.Ticker, &bsv20.Amt)
					if err != nil {
						log.Panic(err)
					}
					txo.Parsed.Bsv20 = bsv20
					txo.Bsv20 = true
				}
			}

			// Save Txo
			result.QueryCount++
			_, err = Db.Exec(`INSERT INTO txos(txid, vout, height, idx, satoshis, outacc, scripthash, lock, listing, bsv20)
				VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT(txid, vout) DO UPDATE SET 
					height=EXCLUDED.height,
					idx=EXCLUDED.idx,
					lock=EXCLUDED.lock,
					listing=EXCLUDED.listing,
					bsv20=EXCLUDED.bsv20`,
				txo.Txid,
				txo.Vout,
				t.Height,
				t.Idx,
				txo.Satoshis,
				txo.OutAcc,
				txo.Scripthash,
				txo.Lock,
				txo.Listing,
				txo.Bsv20,
			)
			if err != nil {
				log.Panicln("insTxo Err:", err)
			}

			if txo.Parsed != nil {
				if txo.Outpoint == txo.Origin {
					result.QueryCount++
					_, err = Db.Exec(`INSERT INTO origins(origin, vout, height, idx, map, search_text_en)
						VALUES($1, $2, $3, $4, $5, search_text_en = to_tsvector('english',
							COALESCE(jsonb_extract_path_text($5, 'name'), '') || ' ' || 
							COALESCE(jsonb_extract_path_text($5, 'description'), '') || ' ' || 
							COALESCE(jsonb_extract_path_text($5, 'subTypeData.description'), '') || ' ' || 
							COALESCE(jsonb_extract_path_text($5, 'keywords'), '')
						))`,
						txo.Origin,
						txo.Vout,
						t.Height,
						t.Idx,
						txo.Parsed.Map,
					)
				}

				if txo.Parsed.Inscription != nil {
					result.QueryCount++
					_, err = Db.Exec(`INSERT INTO inscriptions(txid, vout, height, idx, origin, filehash, filesize, filetype, json_content, sigma)
						VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
						ON CONFLICT(txid, vout) DO UPDATE SET 
							height=EXCLUDED.height, 
							idx=EXCLUDED.idx, 
							origin=EXCLUDED.origin, 
							sigma=EXCLUDED.sigma`,
						txo.Txid,
						txo.Vout,
						t.Height,
						t.Idx,
						txo.Origin,
						txo.Parsed.Inscription.File.Hash,
						txo.Parsed.Inscription.File.Size,
						txo.Parsed.Inscription.File.Type,
						txo.Parsed.Inscription.JsonContent,
						txo.Parsed.Sigmas,
					)
					if err != nil {
						log.Panic(err)
					}
				}

				if txo.Parsed.Map != nil {
					result.QueryCount++
					_, err = Db.Exec(`INSERT INTO map(txid, vout, height, idx, origin, map)
						VALUES($1, $2, $3, $4, $5, $6)
						ON CONFLICT(txid, vout) DO UPDATE SET
							height=EXCLUDED.height,
							idx=EXCLUDED.idx,
							origin=EXCLUDED.origin,
							map=EXCLUDED.map`,
						txo.Txid,
						txo.Vout,
						t.Height,
						t.Idx,
						txo.Origin,
						txo.Parsed.Map,
					)
					if err != nil {
						log.Panicf("%x %v", txo.Txid, err)
					}

					if txo.Origin != nil && txo.Outpoint != txo.Origin {
						result.QueryCount++
						_, err = Db.Exec(`UPDATE origins
							SET map = COALESCE(map, '{}') || $2
							WHERE origin=$1`,
							txo.Origin,
							txo.Parsed.Map,
						)
						if err != nil {
							log.Panic(err)
						}
					}
				}

				if txo.Parsed.B != nil {
					result.QueryCount++
					_, err = Db.Exec(`INSERT INTO b(txid, vout, height, idx, filehash, filesize, filetype, fileenc)
						VALUES($1, $2, $3, $4, $5, $6, $7, $8)
						ON CONFLICT(txid, vout) DO UPDATE SET
							height=EXCLUDED.height,
							idx=EXCLUDED.idx`,
						txo.Txid,
						txo.Vout,
						t.Height,
						t.Idx,
						txo.Parsed.B.Hash,
						txo.Parsed.B.Size,
						txo.Parsed.B.Type,
						txo.Parsed.B.Encoding,
					)
					if err != nil {
						log.Panic(err)
					}
				}

				if txo.Parsed.Listing != nil {
					result.QueryCount++
					_, err = Db.Exec(`INSERT INTO listings(txid, vout, height, idx, origin, price, payout, num, spend, lock, bsv20, map)
						SELECT $1, $2, $3, $4, $5, $6, $7, o.num, t.spend, t.lock, t.bsv20, o.map
						FROM txos t
						JOIN origins o ON o.origin = t.origin
						WHERE t.txid=$1 AND t.vout=$2
						ON CONFLICT(txid, vout) DO UPDATE
							SET height=EXCLUDED.height, 
								idx=EXCLUDED.idx, 
								origin=EXCLUDED.origin,
								lock=EXCLUDED.lock,
								bsv20=EXCLUDED.bsv20,
								map=EXCLUDED.map`,
						txo.Txid,
						txo.Vout,
						t.Height,
						t.Idx,
						txo.Origin,
						txo.Parsed.Listing.Price,
						txo.Parsed.Listing.PayOutput,
					)
					if err != nil {
						log.Panic(err)
					}
					result.Events = append(result.Events, &Event{
						Channel: "list",
						Data:    (*txo.Outpoint)[:],
					})
				}

				if txo.Parsed.Bsv20 != nil {
					b := txo.Parsed.Bsv20
					if b.Op == "deploy" {
						result.QueryCount++
						_, err := Db.Exec(`INSERT INTO bsv20(txid, vout, id, height, idx, tick, max, lim, dec)
							VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
							ON CONFLICT(id) DO UPDATE SET
								height=EXCLUDED.height,
								idx=EXCLUDED.idx`,
							txo.Txid,
							txo.Vout,
							txo.Outpoint,
							t.Height,
							t.Idx,
							b.Ticker,
							b.Max,
							b.Limit,
							b.Decimals,
						)
						if err != nil {
							log.Panic(err)
						}
					}

					result.QueryCount++
					_, err := Db.Exec(`INSERT INTO bsv20_txos(txid, vout, height, idx, tick, op, orig_amt, amt, implied, lock, spend, listing)
						SELECT $1, $2, $3, $4, $5, $6, $7, $7, $8, lock, spend, listing
						FROM txos
						WHERE txid=$1 AND vout=$2
						ON CONFLICT(txid, vout) DO UPDATE SET
							height=EXCLUDED.height,
							idx=EXCLUDED.idx`,
						b.Txid,
						b.Vout,
						t.Height,
						t.Idx,
						b.Ticker,
						b.Op,
						b.Amt,
						b.Implied,
					)
					if err != nil {
						log.Panic(err)
					}
				}
			}
		}
	}
	return
}

func ProcessBlockFees(height uint32) {
	fmt.Printf("Processing Fee Accumulation: %d\n", height)
	_, err := Db.Exec(`SELECT fn_acc_fees($1)`,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
}

// 64ff810301010354786e01ff820001080102547801ff840001024964010a0001054865784964010c000107426c6f636b4964010c00010648656967687401060001034964780106000107506172656e747301ff8e0001084368696c6472656e01ff8e00000042ff8303010102547801ff840001040106496e7075747301ff880001074f75747075747301ff8c00010756657273696f6e01060001084c6f636b54696d6501060000001aff870201010b5b5d2a62742e496e70757401ff880001ff86000076ff85030102ff86000105011250726576696f757354785361746f73686973010600011050726576696f75735478536372697074010a00010f556e6c6f636b696e67536372697074010a00011250726576696f757354784f7574496e646578010600010e53657175656e63654e756d62657201060000001bff8b0201010c5b5d2a62742e4f757470757401ff8c0001ff8a00002bff89030102ff8a00010201085361746f73686973010600010d4c6f636b696e67536372697074010a00000024ff8d040101136d61705b737472696e675d2a6c69622e54786e01ff8e00010c01ff820000fe0122ff82010101031004143909500317e201062f503253482f01fcffffffff01fcffffffff00010101fb012b6fceb80143410454a6a884f0d7db1ee2f362f731cd47881851360ec7647a538437cbdbfe58a31a67671e2caa7baa328b05f2f242da290f6c6618768526601206ed7916c3c61b6bac0001010001202ee88f71e039f93f7f2e5ad06da1b3c4be9294b24999460b2fbd523fe8d1dde501403265653838663731653033396639336637663265356164303664613162336334626539323934623234393939343630623266626435323366653864316464653501403030303030303030303030303033353332396632306565353139656461643033333036653837613730396439303534343061363132376161366138323361313601fd02e5e600
