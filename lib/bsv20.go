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
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Height   uint32     `json:"height"`
	Idx      uint64     `json:"idx"`
	Protocol string     `json:"p"`
	Op       string     `json:"op"`
	Ticker   string     `json:"tick"`
	Id       *Outpoint  `json:"id"`
	Max      uint64     `json:"max"`
	Limit    uint64     `json:"lim"`
	Decimals uint8      `json:"dec"`
	Lock     ByteString `json:"lock"`
	Amt      uint64     `json:"amt"`
	Supply   uint64     `json:"supply"`
	Image    *File      `json:"image"`
}

func parseBsv20(ord *File, height uint32) (*Bsv20, error) {
	if !strings.HasPrefix(ord.Type, "application/bsv-20") &&
		!(height > 0 && height < 793000 && strings.HasPrefix(ord.Type, "text/plain")) {
		return nil, nil
	}
	data := map[string]string{}
	// fmt.Println("JSON:", string(p.Ord.Content))
	err := json.Unmarshal(ord.Content, &data)
	if err != nil {
		// log.Panic(err)
		return nil, err
	}
	if protocol, ok := data["p"]; !ok || protocol != "bsv-20" {
		return nil, nil
	}
	bsv20 := &Bsv20{
		Protocol: data["p"],
		Op:       data["op"],
		Ticker:   data["tick"],
	}
	if amt, ok := data["amt"]; ok {
		bsv20.Amt, err = strconv.ParseUint(amt, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	if max, ok := data["max"]; ok {
		bsv20.Max, err = strconv.ParseUint(max, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	if limit, ok := data["lim"]; ok {
		bsv20.Limit, err = strconv.ParseUint(limit, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	if dec, ok := data["dec"]; ok {
		val, err := strconv.ParseUint(dec, 10, 8)
		if err != nil {
			log.Panic(err)
		}
		if val > 18 {
			return nil, fmt.Errorf("invalid decimals: %d", val)
		}
		bsv20.Decimals = uint8(val)
	} else {
		bsv20.Decimals = 18
	}
	return bsv20, nil
}

func processBsv20Txn(ires *IndexResult) (err error) {
	for _, p := range ires.ParsedScripts {
		if p.Bsv20 == nil {
			continue
		}
		if p.Bsv20.Op == "deploy" {
			p.Bsv20.Id = NewOutpoint(ires.Txid, p.Vout)
			_, err = db.Exec(`INSERT INTO bsv20(id, height, idx, tick, max, lim, dec, map, b)
				VALUES($1, $2, $3, UPPER($4), $5, $6, $7, $8, $9)
				ON CONFLICT(id) DO NOTHING`,
				p.Bsv20.Id,
				ires.Height,
				ires.Idx,
				p.Bsv20.Ticker,
				p.Bsv20.Max,
				p.Bsv20.Limit,
				p.Bsv20.Decimals,
				p.Map,
				p.B,
			)
			if err != nil {
				log.Panic(err)
			}
		}

		// fmt.Println("BSV20:", p.Bsv20.Ticker, p.Bsv20.Amt)
		_, err = db.Exec(`INSERT INTO bsv20_txos(txid, vout, height, idx, tick, op, amt, lock, spend)
			SELECT $1, $2, $3, $4, UPPER($5), $6, $7, $8, spend
			FROM txos
			WHERE txid=$1 AND vout=$2
			ON CONFLICT(txid, vout) DO NOTHING`,
			p.Txid,
			p.Vout,
			ires.Height,
			ires.Idx,
			p.Bsv20.Ticker,
			p.Bsv20.Op,
			p.Bsv20.Amt,
			p.Bsv20.Lock,
		)
		if err != nil {
			log.Panic(err)
		}
	}
	return nil
}

func ValidateBsv20(height uint32) {
	txrows, err := db.Query(`SELECT txid
		FROM bsv20_txos
		WHERE valid IS NULL AND height <= $1
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
		if !valid.Bool {
			setInvalidRollback(t, txid)
			return
		}
		if balance, ok := tokensIn[tick]; ok {
			amt += balance
		}
		tokensIn[tick] = amt
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

	for rows.Next() {
		bsv20 := &Bsv20{}
		err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Op, &bsv20.Ticker, &bsv20.Amt)
		if err != nil {
			log.Panic(err)
		}

		var token *Bsv20
		if t, ok := tokenSupply[bsv20.Ticker]; ok {
			token = t
		} else {
			token = loadBsv20(bsv20.Ticker)
			tokenSupply[bsv20.Ticker] = token
		}

		switch bsv20.Op {
		case "deploy":
			if token != nil {
				setTokenInvalid(t, *NewOutpoint(bsv20.Txid, bsv20.Vout))
				setInvalid(t, txid, bsv20.Vout)
				continue
			}

			bsv20.Id = NewOutpoint(bsv20.Txid, bsv20.Vout)
			_, err = t.Exec(`UPDATE bsv20
				SET valid=TRUE
				WHERE id=$1`,
				bsv20.Id,
			)
			if err != nil {
				fmt.Println(bsv20.Id)
				log.Panic(err)
			}
			setValid(t, bsv20.Txid, bsv20.Vout)
		case "mint":
			if token == nil || bsv20.Amt > token.Limit {
				log.Panicf("invalid mint: %s %d %d %d %d %d %v", bsv20.Ticker, bsv20.Height, bsv20.Idx, bsv20.Amt, bsv20.Max, bsv20.Limit, token)
				setInvalidRollback(t, txid)
				return
			}

			if token.Supply >= token.Max {
				log.Panicf("invalid supply: %s %d %d", bsv20.Ticker, token.Supply, token.Max)
				setInvalid(t, txid, bsv20.Vout)
				continue
			}

			if token.Supply+bsv20.Amt > token.Max {
				bsv20.Amt = bsv20.Max - token.Supply
			}

			token.Supply += bsv20.Amt
			_, err := t.Exec(`UPDATE bsv20
				SET supply=$2
				WHERE id=$1`,
				token.Id,
				token.Supply,
			)
			if err != nil {
				log.Panic(err)
			}
			setValid(t, bsv20.Txid, bsv20.Vout)

		case "transfer":

			if balance, ok := tokensIn[bsv20.Ticker]; ok {
				if balance < bsv20.Amt {
					log.Fatalf("invalid transfer: %s %d %d", bsv20.Ticker, balance, bsv20.Amt)
					setInvalidRollback(t, txid)
					return
				}
				balance -= bsv20.Amt
				tokensIn[bsv20.Ticker] = balance
				setValid(t, bsv20.Txid, bsv20.Vout)
			} else {
				log.Fatalf("invalid transfer: %s %d %d", bsv20.Ticker, balance, bsv20.Amt)
				setInvalidRollback(t, txid)
				return
			}
		}
	}
	rows.Close()
	err = t.Commit()
	fmt.Printf("Commit BSV20 %x\n", txid)
	if err != nil {
		log.Panic(err)
	}
	return
}

func loadBsv20(tick string) (bsv20 *Bsv20) {
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
		bsv20 = &Bsv20{}
		err = rows.Scan(&bsv20.Id, &bsv20.Ticker, &bsv20.Max, &bsv20.Limit, &bsv20.Supply)
		if err != nil {
			log.Panic(err)
		}
	}
	return
}

func setValid(t *sql.Tx, txid []byte, vout uint32) {
	_, err := t.Exec(`UPDATE bsv20_txos
		SET valid=TRUE
		WHERE txid=$1 AND vout=$2`,
		txid,
		vout,
	)
	if err != nil {
		log.Panic(err)
	}
}

func setTokenInvalid(t *sql.Tx, id []byte) bool {
	_, err := t.Exec(`UPDATE bsv20
		SET valid=FALSE
		WHERE id=$1`,
		id,
	)
	if err != nil {
		log.Panic(err)
	}
	return true
}

func setInvalid(t *sql.Tx, txid []byte, vout uint32) {
	_, err := db.Exec(`UPDATE bsv20_txos
		SET valid=FALSE
		WHERE txid=$1 AND vout=$2`,
		txid,
		vout,
	)
	if err != nil {
		log.Panic(err)
	}
}

func setInvalidRollback(t *sql.Tx, txid []byte) {
	err := t.Rollback()
	if err != nil {
		log.Panic(err)
	}
	_, err = db.Exec(`UPDATE bsv20_txos
		SET valid=FALSE
		WHERE txid=$1`,
		txid,
	)
	if err != nil {
		log.Panic(err)
	}
}
