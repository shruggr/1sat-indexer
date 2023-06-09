package lib

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
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
	Listing  bool         `json:"listing"`
	Valid    sql.NullBool `json:"valid"`
	Reason   string       `json:"reason"`
}

func parseBsv20(ord *File, height uint32) (bsv20 *Bsv20, err error) {
	mime := strings.ToLower(ord.Type)
	if !strings.HasPrefix(mime, "application/bsv-20") &&
		!(height > 0 && height < 793000 && strings.HasPrefix(mime, "text/plain")) {
		return nil, nil
	}
	data := map[string]string{}
	// fmt.Println("JSON:", string(p.Ord.Content))
	err = json.Unmarshal(ord.Content, &data)
	if err != nil {
		fmt.Println("JSON PARSE ERROR:", err)
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
			// bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			// bsv20.Reason = "amt parse"
			return nil, nil
		}
	}
	if max, ok := data["max"]; ok {
		bsv20.Max, err = strconv.ParseUint(max, 10, 64)
		if err != nil {
			// bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			// bsv20.Reason = "max parse"
			return nil, nil
		}
	}
	if limit, ok := data["lim"]; ok {
		bsv20.Limit, err = strconv.ParseUint(limit, 10, 64)
		if err != nil {
			// bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			// bsv20.Reason = "lim parse"
			return nil, nil
		}
	}
	if dec, ok := data["dec"]; ok {
		var val uint64
		val, err = strconv.ParseUint(dec, 10, 8)
		if err != nil {
			// bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			// bsv20.Reason = "dec parse"
			return nil, nil
		} else if val > 18 {
			// bsv20.Valid = sql.NullBool{Bool: false, Valid: true}
			// bsv20.Reason = fmt.Sprintf("dec %s > 18", dec)
			return nil, nil
		}
		bsv20.Decimals = uint8(val)
	} else {
		bsv20.Decimals = 18
	}
	return bsv20, nil
}

func (b *Bsv20) Save() {
	b.Ticker = strings.ToUpper(b.Ticker)
	b.Op = strings.ToLower(b.Op)
	if b.Op == "deploy" {
		b.Id = NewOutpoint(b.Txid, b.Vout)
		_, err := db.Exec(`INSERT INTO bsv20(txid, vout, id, height, idx, tick, max, lim, dec, map, b, valid, reason)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT(id) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				max=EXCLUDED.max,
				lim=EXCLUDED.lim`,
			b.Txid,
			b.Vout,
			b.Id,
			b.Height,
			b.Idx,
			b.Ticker,
			b.Max,
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

	_, err := db.Exec(`INSERT INTO bsv20_txos(txid, vout, height, idx, tick, op, amt, orig_amt, lock, implied, spend, valid, reason, listing)
		SELECT $1, $2, $3, $4, UPPER($5), $6, $7, $7, lock, $8, spend, $9, $10, listing
		FROM txos
		WHERE txid=$1 AND vout=$2
		ON CONFLICT(txid, vout) DO UPDATE SET
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			implied=EXCLUDED.implied,
			lock=EXCLUDED.lock,
			amt=EXCLUDED.amt,
			orig_amt=EXCLUDED.orig_amt,
			listing=EXCLUDED.listing`,
		b.Txid,
		b.Vout,
		b.Height,
		b.Idx,
		b.Ticker,
		b.Op,
		b.Amt,
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
		var amt uint64
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
			Amt:     amt,
			Lock:    txo.Lock,
			Implied: true,
		}
		bsv20.Save()
	}
}

func ValidateBsv20(height uint32) {
	rows, err := db.Query(`SELECT DISTINCT tick
		FROM bsv20_txos
		WHERE valid IS NULL AND height <= $1 AND height > 0`,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	// tokenSupply = map[string]*Bsv20{}
	// processed := map[string]struct{}{}
	threadLimiter := make(chan struct{}, THREADS)
	var wg sync.WaitGroup
	for rows.Next() {
		var tick string
		err = rows.Scan(&tick)
		if err != nil {
			log.Panic(err)
		}
		// if _, ok := processed[hex.EncodeToString(txid)]; !ok {
		// 	processed[hex.EncodeToString(txid)] = struct{}{}
		// }
		// ValidateTxBsv20s(txid, false)
		threadLimiter <- struct{}{}
		wg.Add(1)
		go func(tick string) {
			ValidateTicker(height, tick)
			wg.Done()
			<-threadLimiter
		}(tick)
	}
	wg.Wait()
}

type Bsv20Results struct {
	Balance   map[string]uint64
	TokensIn  []*Bsv20
	TokensOut []*Bsv20
	Tickers   []*Bsv20
	Reasons   map[uint32]string
}
type OpResult struct {
	Valid   uint
	Invalid uint
}
type TickerResults struct {
	Height   uint32
	Deploy   OpResult
	Mint     OpResult
	Transfer OpResult
}

// var tokenSupply map[string]*Bsv20

func ValidateTicker(height uint32, tick string) (r *TickerResults) {
	ticker := loadTicker(tick)
	r = &TickerResults{
		Height: height,
	}

	tickRows, err := db.Query(`SELECT txid, vout, height, idx, op, orig_amt
		FROM bsv20_txos
		WHERE tick=$1 AND valid IS NULL AND height <= $2 AND height > 0
		ORDER BY op, height, idx`,
		tick,
		height,
	)
	if err != nil {
		log.Panicln(err)
	}

	t, err := db.Begin()
	if err != nil {
		log.Panic(err)
	}
	defer t.Rollback()
	invalidTransfers := map[string]string{}
	tokensIn := map[string]uint64{}

	for tickRows.Next() {
		bsv20 := &Bsv20{}
		err = tickRows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Op, &bsv20.Amt)
		if err != nil {
			log.Panic(err)
		}

		reason := ""
		if bsv20.Op == "deploy" {
			outpoint := NewOutpoint(bsv20.Txid, bsv20.Vout)
			if ticker != nil {
				reason = "duplicate"
				setTokenInvalid(t, *outpoint, reason)
				setInvalid(t, bsv20.Txid, bsv20.Vout, reason)
				r.Deploy.Invalid++
				continue
			}
			chars := []rune(tick)
			if len(chars) > 4 {
				reason = fmt.Sprintf("length %d", len(chars))
				setTokenInvalid(t, *outpoint, reason)
				setInvalid(t, bsv20.Txid, bsv20.Vout, reason)
				r.Deploy.Invalid++
				continue
			}

			row := t.QueryRow(`UPDATE bsv20 SET valid=TRUE 
				WHERE id=$1
				RETURNING id, height, idx, tick, max, lim, supply`,
				outpoint,
			)
			ticker = &Bsv20{}
			var maxInt int64
			var supplyInt int64
			var limitInt int64
			err = row.Scan(&ticker.Id, &ticker.Height, &ticker.Idx, &ticker.Ticker, &maxInt, &limitInt, &supplyInt)
			if err != nil {
				log.Panicln(outpoint, err)
			}
			ticker.Max = uint64(maxInt)
			ticker.Supply = uint64(supplyInt)
			ticker.Limit = uint64(limitInt)
			setValid(t, bsv20.Txid, bsv20.Vout, "")
			r.Deploy.Valid++
			// ticker = loadTicker(tick)
			continue
		}

		if bsv20.Amt == 0 {
			setInvalid(t, bsv20.Txid, bsv20.Vout, fmt.Sprintf("%s amt: 0", bsv20.Op))
			switch bsv20.Op {
			case "mint":
				r.Mint.Invalid++
			case "transfer":
				r.Transfer.Invalid++
			}
			continue
		}

		switch bsv20.Op {
		case "mint":
			if ticker == nil || ticker.Height > bsv20.Height || (ticker.Height == bsv20.Height && ticker.Idx > bsv20.Idx) {
				reason = fmt.Sprintf("invalid ticker %s as of %d %d", tick, bsv20.Height, bsv20.Idx)
			} else if ticker.Supply >= ticker.Max {
				reason = fmt.Sprintf("supply %d >= max %d", ticker.Supply, ticker.Max)
			} else if ticker.Limit > 0 && bsv20.Amt > ticker.Limit {
				reason = fmt.Sprintf("amt %d > limit %d", bsv20.Amt, ticker.Limit)
			}
			if reason != "" {
				// fmt.Println("REASON:", reason)
				setInvalid(t, bsv20.Txid, bsv20.Vout, reason)
				r.Mint.Invalid++
				continue
			}

			if ticker.Max-ticker.Supply < bsv20.Amt {
				reason = fmt.Sprintf("supply %d + amt %d > max %d", ticker.Supply, bsv20.Amt, ticker.Max)
				bsv20.Amt = ticker.Max - ticker.Supply
			}
			_, err := t.Exec(`UPDATE bsv20_txos
				SET amt=$3, valid=TRUE, reason=$4
				WHERE txid=$1 AND vout=$2`,
				bsv20.Txid,
				bsv20.Vout,
				bsv20.Amt,
				reason,
			)
			if err != nil {
				log.Panic(err)
			}
			ticker.Supply += bsv20.Amt
			r.Mint.Valid++
		case "transfer":
			if ticker == nil || ticker.Height > bsv20.Height || (ticker.Height == bsv20.Height && ticker.Idx > bsv20.Idx) {
				setInvalid(t, bsv20.Txid, bsv20.Vout, fmt.Sprintf("invalid ticker %s as of %d %d", tick, bsv20.Height, bsv20.Idx))
				r.Transfer.Invalid++
				continue
			}
			txid := hex.EncodeToString(bsv20.Txid)
			if _, ok := invalidTransfers[txid]; ok {
				setInvalid(t, bsv20.Txid, bsv20.Vout, "invalid transfer")
				r.Transfer.Invalid++
				continue
			}
			var balance uint64
			if tbal, ok := tokensIn[txid]; ok {
				balance = tbal
			} else {
				rows, err := t.Query(`SELECT amt, valid
					FROM bsv20_txos
					WHERE spend=$1 AND valid=TRUE`,
					bsv20.Txid,
				)
				if err != nil {
					log.Panicln(err)
				}
				for rows.Next() {
					var amt int64
					var valid sql.NullBool
					err = rows.Scan(&amt, &valid)
					if err != nil {
						log.Panicln(err)
					}
					if !valid.Bool {
						reason = "invalid input"
						rows.Close()
						break
					}
					balance += uint64(amt)
				}
				rows.Close()
			}
			if balance < bsv20.Amt {
				reason = fmt.Sprintf("insufficient inputs: bal %d < amt %d", balance, bsv20.Amt)
				_, err := t.Exec(`UPDATE bsv20_txos
					SET VALID=FALSE, reason=$3
					WHERE txid=$1 AND tick=$2`,
					bsv20.Txid,
					tick,
					reason,
				)
				if err != nil {
					log.Panicln(err)
				}
				invalidTransfers[txid] = reason
				r.Transfer.Invalid++
				continue
			}
			reason = fmt.Sprintf("amt %d <= bal %d", bsv20.Amt, balance)
			balance -= bsv20.Amt
			tokensIn[txid] = balance
			setValid(t, bsv20.Txid, bsv20.Vout, reason)
			r.Transfer.Valid++
		default:
			setInvalid(t, bsv20.Txid, bsv20.Vout, fmt.Sprintf("invalid op: %s", bsv20.Op))
		}
	}
	tickRows.Close()
	if ticker != nil {
		_, err = t.Exec(`UPDATE bsv20
			SET supply=$2
			WHERE id=$1`,
			ticker.Id,
			ticker.Supply,
		)
	}
	if err != nil {
		log.Panic(err)
	}
	err = t.Commit()
	fmt.Printf("BSV20 %s - dep: %d %d mint: %d %d xfer: %d %d\n", tick, r.Deploy.Valid, r.Deploy.Invalid, r.Mint.Valid, r.Mint.Invalid, r.Transfer.Valid, r.Transfer.Invalid)
	if err != nil {
		log.Panic(err)
	}
	return
}

func loadTicker(tick string) (ticker *Bsv20) {
	rows, err := db.Query(`SELECT id, height, idx, tick, max, lim, supply
		FROM bsv20
		WHERE tick=$1 AND valid=TRUE`,
		tick,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		ticker = &Bsv20{}
		var maxInt int64
		var supplyInt int64
		var limitInt int64
		err = rows.Scan(&ticker.Id, &ticker.Height, &ticker.Idx, &ticker.Ticker, &maxInt, &limitInt, &supplyInt)
		if err != nil {
			log.Panicln(tick, err)
		}

		ticker.Max = uint64(maxInt)
		ticker.Supply = uint64(supplyInt)
		ticker.Limit = uint64(limitInt)
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

// func setTickerInvalid(t *sql.Tx, tick string, reason string) {
// 	_, err := t.Exec(`UPDATE bsv20
// 		SET valid=FALSE, reason=$2
// 		WHERE tick=$1 AND valid IS NULL`,
// 		tick,
// 		reason,
// 	)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// }

// func setTxosInvalid(t *sql.Tx, tick string, reason string) {
// 	_, err := t.Exec(`UPDATE bsv20_txos
// 		SET valid=FALSE, reason=$3
// 		WHERE tick=$1 AND valid IS NULL`,
// 		tick,
// 		reason,
// 	)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// }

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
