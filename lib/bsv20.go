package lib

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
)

type Bsv20 struct {
	Txid     ByteString   `json:"txid"`
	Vout     uint32       `json:"vout"`
	Height   uint32       `json:"height"`
	Idx      uint64       `json:"idx"`
	Protocol string       `json:"p"`
	Op       string       `json:"op"`
	Ticker   string       `json:"tick"`
	Id       *Outpoint    `json:"id"`
	Max      uint64       `json:"max"`
	Limit    uint64       `json:"lim"`
	Decimals uint8        `json:"dec"`
	Lock     ByteString   `json:"lock"`
	Amt      uint64       `json:"amt"`
	Supply   uint64       `json:"supply"`
	Map      Map          `json:"MAP,omitempty"`
	B        *File        `json:"B,omitempty"`
	Implied  bool         `json:"implied"`
	Valid    sql.NullBool `json:"valid"`
	Reason   string       `json:"reason"`
}

func parseBsv20(ord *File, height uint32) (bsv20 *Bsv20, err error) {
	if !strings.HasPrefix(ord.Type, "application/bsv-20") &&
		!(height > 0 && height < 793000 && strings.HasPrefix(ord.Type, "text/plain")) {
		return nil, nil
	}
	data := map[string]string{}
	// fmt.Println("JSON:", string(p.Ord.Content))
	err = json.Unmarshal(ord.Content, &data)
	if err != nil {
		return
	}
	if protocol, ok := data["p"]; !ok || protocol != "bsv-20" {
		return nil, nil
	}
	bsv20 = &Bsv20{
		Protocol: data["p"],
		Op:       data["op"],
		Ticker:   data["tick"],
	}
	if amt, ok := data["amt"]; ok {
		bsv20.Amt, err = strconv.ParseUint(amt, 10, 64)
		if err != nil {
			bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			bsv20.Reason = "amt parse"
			return
		}
	}
	if max, ok := data["max"]; ok {
		bsv20.Max, err = strconv.ParseUint(max, 10, 64)
		if err != nil {
			bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			bsv20.Reason = "max parse"
			return
		}
	}
	if limit, ok := data["lim"]; ok {
		bsv20.Limit, err = strconv.ParseUint(limit, 10, 64)
		if err != nil {
			bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			bsv20.Reason = "lim parse"
			return
		}
	}
	if dec, ok := data["dec"]; ok {
		var val uint64
		val, err = strconv.ParseUint(dec, 10, 8)
		if err != nil {
			bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			bsv20.Reason = "dec parse"
			return
		} else if val > 18 {
			bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			bsv20.Reason = fmt.Sprintf("dec %s > 18", dec)
			return
		}
		bsv20.Decimals = uint8(val)
	} else {
		bsv20.Decimals = 18
	}
	return bsv20, nil
}

func (b *Bsv20) Save() {
	if b.Op == "deploy" {
		b.Id = NewOutpoint(b.Txid, b.Vout)
		_, err := db.Exec(`INSERT INTO bsv20(id, height, idx, tick, max, lim, dec, map, b, valid, reason)
			VALUES($1, $2, $3, UPPER($4), $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT(id) DO NOTHING`,
			b.Id,
			b.Height,
			b.Idx,
			b.Ticker,
			int64(b.Max),
			b.Limit,
			b.Decimals,
			b.Map,
			b.B,
			b.Valid,
			b.Reason,
		)
		if err != nil {
			log.Panic(err)
		}
	}

	_, err := db.Exec(`INSERT INTO bsv20_txos(txid, vout, height, idx, tick, op, amt, lock, implied, spend, valid, reason)
		SELECT $1, $2, $3, $4, UPPER($5), $6, $7, $8, $9, spend, $10, $11
		FROM txos
		WHERE txid=$1 AND vout=$2
		ON CONFLICT(txid, vout) DO UPDATE SET
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			implied=EXCLUDED.implied,
			lock=EXCLUDED.lock`,
		b.Txid,
		b.Vout,
		b.Height,
		b.Idx,
		b.Ticker,
		b.Op,
		int64(b.Amt),
		b.Lock,
		b.Implied,
		b.Valid,
		b.Reason,
	)
	if err != nil {
		log.Panic(err)
	}
}

func saveImpliedBsv20Transfer(txid []byte, vout uint32, txo *Txo) {
	rows, err := db.Query(`SELECT tick, amt
		FROM bsv20_txos
		WHERE txid=$1 AND vout=$2`,
		txid,
		vout,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		var ticker string
		var amt int64
		err := rows.Scan(&ticker, &amt)
		if err != nil {
			log.Panic(err)
		}
		bsv20 := &Bsv20{
			Txid:    txo.Txid,
			Vout:    txo.Vout,
			Height:  txo.Height,
			Idx:     txo.Idx,
			Op:      "transfer",
			Ticker:  ticker,
			Amt:     uint64(amt),
			Lock:    txo.Lock,
			Implied: true,
		}
		bsv20.Save()
	}
}

func ValidateBsv20(height uint32) {
	txrows, err := db.Query(`SELECT txid
		FROM bsv20_txos
		WHERE valid IS NULL AND height <= $1 AND height > 0
		ORDER BY height ASC, idx ASC`,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer txrows.Close()

	processed := map[string]struct{}{}
	for txrows.Next() {
		var txid []byte
		err = txrows.Scan(&txid)
		if err != nil {
			log.Panic(err)
		}
		if _, ok := processed[hex.EncodeToString(txid)]; !ok {
			validateTxBsv20s(txid)
			processed[hex.EncodeToString(txid)] = struct{}{}
		}

	}
}

func validateTxBsv20s(txid []byte) (updates int64) {
	rows, err := db.Query(`SELECT tick, amt, valid
			FROM bsv20_txos
			WHERE spend=$1`,
		txid,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()

	t, err := db.Begin()
	if err != nil {
		log.Panic(err)
	}
	defer t.Rollback()
	tokensIn := map[string]uint64{}
	for rows.Next() {
		var amt uint64
		var tick string
		var valid sql.NullBool
		err = rows.Scan(&tick, &amt, &valid)
		if err != nil {
			log.Panicln(err)
		}
		if valid.Valid && valid.Bool {
			if balance, ok := tokensIn[tick]; ok {
				amt += balance
			}
			tokensIn[tick] = amt
		}
	}

	rows, err = db.Query(`SELECT txid, vout, height, idx, op, tick, amt
		FROM bsv20_txos
		WHERE txid=$1
		ORDER BY vout ASC`,
		txid,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	tokenSupply := map[string]*Bsv20{}

	invalidTransfers := map[string]struct{}{}
	for rows.Next() {
		bsv20 := &Bsv20{}
		err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Op, &bsv20.Ticker, &bsv20.Amt)
		if err != nil {
			log.Panic(err)
		}

		var ticker *Bsv20
		if t, ok := tokenSupply[bsv20.Ticker]; ok {
			ticker = t
		} else {
			ticker = loadTicker(bsv20.Ticker)
			tokenSupply[bsv20.Ticker] = ticker
		}

		switch bsv20.Op {
		case "deploy":
			chars := []rune(bsv20.Ticker)
			reason := ""
			if ticker != nil {
				reason = "duplicate"
			} else if len(chars) > 4 {
				reason = fmt.Sprintf("length %d", len(chars))
			}
			if reason != "" {
				setTokenInvalid(t, *NewOutpoint(bsv20.Txid, bsv20.Vout), reason)
				setInvalid(t, txid, bsv20.Vout, reason)
				continue
			}

			bsv20.Id = NewOutpoint(bsv20.Txid, bsv20.Vout)
			_, err = t.Exec(`UPDATE bsv20
				SET valid=TRUE
				WHERE id=$1`,
				bsv20.Id,
			)
			if err != nil {
				log.Panicln(bsv20.Id, err)
			}
			setValid(t, bsv20.Txid, bsv20.Vout, "")
		case "mint":
			reason := ""
			if ticker == nil {
				reason = fmt.Sprintf("invalid ticker %s", bsv20.Ticker)
			} else if ticker.Supply >= ticker.Max {
				reason = fmt.Sprintf("supply %d = max %d", ticker.Supply, ticker.Max)
			} else if ticker.Limit > 0 && bsv20.Amt > ticker.Limit {
				reason = fmt.Sprintf("amt %d > limit %d", bsv20.Amt, ticker.Limit)
			}
			if reason != "" {
				setInvalid(t, txid, bsv20.Vout, reason)
				continue
			}

			if ticker.Supply+bsv20.Amt > ticker.Max {
				bsv20.Amt = bsv20.Max - ticker.Supply
				reason = fmt.Sprintf("supply %d + amt %d > max %d", ticker.Supply, bsv20.Amt, ticker.Max)
			}

			ticker.Supply += bsv20.Amt
			_, err := t.Exec(`UPDATE bsv20
				SET supply=$2
				WHERE id=$1`,
				ticker.Id,
				ticker.Supply,
			)
			if err != nil {
				log.Panic(err)
			}
			setValid(t, bsv20.Txid, bsv20.Vout, reason)

		case "transfer":
			reason := ""
			if _, ok := invalidTransfers[bsv20.Ticker]; ok {
				reason = "invalid transfer"
			}
			var balance uint64
			if tbal, ok := tokensIn[bsv20.Ticker]; ok {
				if bsv20.Amt > balance {
					reason = fmt.Sprintf("amt %d > bal %d", bsv20.Amt, balance)
				} else {
					balance = tbal
				}
			} else {
				reason = "no inputs"
			}
			if reason != "" {
				setInvalid(t, bsv20.Txid, bsv20.Vout, reason)
				continue
			}
			balance -= bsv20.Amt
			tokensIn[bsv20.Ticker] = balance
			setValid(t, bsv20.Txid, bsv20.Vout, fmt.Sprintf("amt %d <= bal %d", bsv20.Amt, balance))
		}
	}
	rows.Close()
	err = t.Commit()
	fmt.Printf("BSV20 %x\n", txid)
	if err != nil {
		log.Panic(err)
	}
	return
}

func loadTicker(tick string) (ticker *Bsv20) {
	rows, err := db.Query(`SELECT id, tick, max, lim, supply
		FROM bsv20
		WHERE tick=UPPER($1) AND valid=TRUE`,
		tick,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		ticker = &Bsv20{}
		err = rows.Scan(&ticker.Id, &ticker.Ticker, &ticker.Max, &ticker.Limit, &ticker.Supply)
		if err != nil {
			log.Panic(err)
		}
	}
	return
}

func setValid(t *sql.Tx, txid []byte, vout uint32, reason string) {
	_, err := t.Exec(`UPDATE bsv20_txos
		SET valid=TRUE, reason=$3
		WHERE txid=$1 AND vout=$2`,
		txid,
		vout,
		reason,
	)
	if err != nil {
		log.Panic(err)
	}
}

func setTokenInvalid(t *sql.Tx, id []byte, reason string) bool {
	_, err := t.Exec(`UPDATE bsv20
		SET valid=FALSE, reason=$2
		WHERE id=$1`,
		id,
		reason,
	)
	if err != nil {
		log.Panic(err)
	}
	return true
}

func setInvalid(t *sql.Tx, txid []byte, vout uint32, reason string) {
	_, err := db.Exec(`UPDATE bsv20_txos
		SET valid=FALSE, reason=$3
		WHERE txid=$1 AND vout=$2`,
		txid,
		vout,
		reason,
	)
	if err != nil {
		log.Panic(err)
	}
}

// func setInvalidTransfer(t *sql.Tx, txid []byte, ticker string, reason string) {
// 	_, err := db.Exec(`UPDATE bsv20_txos
// 		SET valid=FALSE, reason=$3
// 		WHERE txid=$1 AND ticker=$2 AND op='transfer'`,
// 		txid,
// 		ticker,
// 		reason,
// 	)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// }
