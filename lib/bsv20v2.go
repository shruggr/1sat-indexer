package lib

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

type Bsv20Status int

const (
	Invalid Bsv20Status = -1
	Pending Bsv20Status = 0
	Valid   Bsv20Status = 1
)

// type Bsv20Deploy struct {
// 	Protocol string      `json:"p"`
// 	Op       string      `json:"op"`
// 	Ticker   string      `json:"tick"`
// 	Status   Bsv20Status `json:"status"`
// 	Reason   string      `json:"reason"`
// }

type Bsv20 struct {
	Txid     []byte      `json:"-"`
	Vout     uint32      `json:"-"`
	Height   *uint32     `json:"-"`
	Idx      uint64      `json:"-"`
	Ticker   *string     `json:"tick,omitempty"`
	Id       *Outpoint   `json:"id"`
	Op       string      `json:"op"`
	Max      uint64      `json:"-"`
	Limit    uint64      `json:"-"`
	Decimals uint8       `json:"-"`
	Supply   uint64      `json:"-"`
	Amt      *uint64     `json:"amt"`
	Implied  bool        `json:"implied,omitempty"`
	Status   Bsv20Status `json:"status"`
	Reason   *string     `json:"reason,omitempty"`
}

func parseBsv20(ord *File, height *uint32) (bsv20 *Bsv20, err error) {
	mime := strings.ToLower(ord.Type)
	if !strings.HasPrefix(mime, "application/bsv-20") &&
		!(height != nil && *height < 793000 && strings.HasPrefix(mime, "text/plain")) {
		return nil, nil
	}
	data := map[string]string{}
	err = json.Unmarshal(ord.Content, &data)
	if err != nil {
		// fmt.Println("JSON PARSE ERROR:", ord.Content, err)
		return
	}
	var protocol string
	var ok bool
	if protocol, ok = data["p"]; !ok || protocol != "bsv-20" {
		return nil, nil
	}
	bsv20 = &Bsv20{}
	if op, ok := data["op"]; ok {
		bsv20.Op = strings.ToLower(op)
	} else {
		return nil, nil
	}

	if tick, ok := data["tick"]; ok {
		chars := []rune(tick)
		if len(chars) > 4 {
			return nil, nil
		}
		tick = strings.ToUpper(tick)
		bsv20.Ticker = &tick
	}

	if id, ok := data["id"]; ok {
		bsv20.Id, err = NewOutpointFromString(id)
		if err != nil {
			return nil, nil
		}
	}

	switch bsv20.Op {
	case "deploy":
		if max, ok := data["max"]; ok {
			bsv20.Max, err = strconv.ParseUint(max, 10, 64)
			if err != nil {
				return nil, nil
			}
		}
		if limit, ok := data["lim"]; ok {
			bsv20.Limit, err = strconv.ParseUint(limit, 10, 64)
			if err != nil {
				return nil, nil
			}
		}
	case "deploy+mint":
		bsv20.Status = Valid
	case "mint":
		if bsv20.Ticker == nil {
			return nil, nil
		}
	case "transfer":
		if bsv20.Ticker == nil && bsv20.Id == nil {
			return nil, nil
		}
	default:
		return nil, nil
	}

	if amt, ok := data["amt"]; ok {
		amt, err := strconv.ParseUint(amt, 10, 64)
		if err != nil {
			return nil, nil
		}
		bsv20.Amt = &amt
	}

	if dec, ok := data["dec"]; ok {
		var val uint64
		val, err = strconv.ParseUint(dec, 10, 8)
		if err != nil || val > 18 {
			return nil, nil
		}
		bsv20.Decimals = uint8(val)
	}

	return bsv20, nil
}

func SaveBsv20(t *Txo) {
	b := t.Data.Bsv20
	if b.Op == "deploy" {
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20(txid, vout, height, idx, tick, max, lim, dec, status, reason)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT(txid, vout) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			t.Txid,
			t.Vout,
			t.Height,
			t.Idx,
			b.Ticker,
			strconv.FormatUint(b.Max, 10),
			b.Limit,
			b.Decimals,
			b.Status,
			b.Reason,
		)
		if err != nil {
			log.Panic(err)
		}
	} else if b.Op == "mint" {
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20_mints(txid, vout, height, idx, tick, amt)
			VALUES($1, $2, $3, $4, $5, $6)
			ON CONFLICT(txid, vout) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			t.Txid,
			t.Vout,
			t.Height,
			t.Idx,
			b.Ticker,
			b.Amt,
		)
		if err != nil {
			log.Panic(err)
		}
	}
}

func ValidateBsv20(height uint32) {
	fmt.Println("Validating BSV20", height)
	validateBsv20Deploy(height - 6)
	validateBsv20Mint(height - 6)
	validateBsv20Transfers(height)
}

func validateBsv20Deploy(height uint32) {
	rows, err := Db.Query(context.Background(), `
		SELECT txid, vout, height, idx, tick, max, lim, supply
		FROM bsv20
		WHERE status=0 AND height <= $1 AND height IS NOT NULL
		ORDER BY height ASC, idx ASC, vout ASC`,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		ticker := &Bsv20{}
		err = rows.Scan(&ticker.Txid, &ticker.Vout, &ticker.Height, &ticker.Idx, &ticker.Ticker, &ticker.Max, &ticker.Limit, &ticker.Supply)
		if err != nil {
			log.Panic(err)
		}

		func(bsv20 *Bsv20) {
			rows, err := Db.Query(context.Background(), `
				SELECT txid, vout
				FROM bsv20
				WHERE tick=$1 AND status=1 AND (
					height < $2 OR
					(height = $2 AND idx < $3) OR 
					(height = $2 AND idx = $3 AND vout < $4)
				)
				LIMIT 1`,
				bsv20.Ticker,
				bsv20.Height,
				bsv20.Idx,
				bsv20.Vout,
			)
			if err != nil {
				log.Panic(err)
			}
			defer rows.Close()

			t, err := Db.Begin(context.Background())
			if err != nil {
				log.Panic(err)
			}
			defer t.Rollback(context.Background())

			if rows.Next() {
				_, err = t.Exec(context.Background(), `
					UPDATE bsv20
					SET status = -1, reason='duplicate'
					WHERE txid = $1 AND vout = $2`,
					bsv20.Txid,
					bsv20.Vout,
				)
				if err != nil {
					log.Panic(err)
				}

				setInvalid(&t, bsv20.Txid, bsv20.Vout, "duplicate")
				err = t.Commit(context.Background())
				if err != nil {
					log.Panic(err)
				}
				return
			}
			_, err = t.Exec(context.Background(), `
				UPDATE bsv20
				SET status = 1
				WHERE txid = $1 AND vout = $2`,
				bsv20.Txid,
				bsv20.Vout,
			)
			if err != nil {
				log.Panic(err)
			}
			setValid(&t, bsv20.Txid, bsv20.Vout, "")
			err = t.Commit(context.Background())
			if err != nil {
				log.Panic(err)
			}
		}(ticker)
	}
}

func validateBsv20Mint(height uint32) {
	fmt.Println("Validating BSV20 mint", height)
	rows, err := Db.Query(context.Background(), `
		SELECT txid, vout, height, idx, tick, amt
		FROM bsv20_mints
		WHERE status=0 AND height <= $1 AND height > 0
		ORDER BY height ASC, idx ASC, vout ASC`,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	tickers := map[string]*Bsv20{}
	for rows.Next() {
		bsv20 := &Bsv20{}
		err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Ticker, &bsv20.Amt)
		if err != nil {
			log.Panic(err)
		}
		var ticker *Bsv20
		var ok bool
		tick := *bsv20.Ticker
		if ticker, ok = tickers[tick]; !ok {
			ticker = loadTicker(tick)
			tickers[tick] = ticker
		}

		// fmt.Println("TICKER:", *ticker.Ticker, ticker.Supply, *bsv20.Amt)

		var reason string
		if ticker == nil || *ticker.Height > *bsv20.Height || (*ticker.Height == *bsv20.Height && ticker.Idx > bsv20.Idx) {
			reason = fmt.Sprintf("invalid ticker %s as of %d %d", tick, *bsv20.Height, bsv20.Idx)
		} else if ticker.Supply >= ticker.Max {
			reason = fmt.Sprintf("supply %d >= max %d", ticker.Supply, ticker.Max)
		} else if ticker.Limit > 0 && *bsv20.Amt > ticker.Limit {
			reason = fmt.Sprintf("amt %d > limit %d", *bsv20.Amt, ticker.Limit)
		}

		func() {
			t, err := Db.Begin(context.Background())
			if err != nil {
				log.Panic(err)
			}
			defer t.Rollback(context.Background())

			if reason != "" {
				_, err = t.Exec(context.Background(), `
					UPDATE bsv20_mints
					SET status=-1, reason=$3
					WHERE txid = $1 AND vout=$2`,
					bsv20.Txid,
					bsv20.Vout,
					reason,
				)
				if err != nil {
					log.Panic(err)
				}
				sql := fmt.Sprintf(`
					UPDATE txos
					SET data = jsonb_set(data, '{bsv20}', data->'bsv20' || '{"status": -1, "reason":"%s"}')
					WHERE txid = $1 AND vout = $2`,
					reason,
				)
				_, err := t.Exec(context.Background(),
					sql,
					bsv20.Txid,
					bsv20.Vout,
				)
				if err != nil {
					log.Panic(err)
				}
				err = t.Commit(context.Background())
				if err != nil {
					log.Panic(err)
				}
				return
			}

			var sql string
			var reason string
			if ticker.Max-ticker.Supply < *bsv20.Amt {
				*bsv20.Amt = ticker.Max - ticker.Supply
				reason = fmt.Sprintf("supply %d + amt %d > max %d", ticker.Supply, *bsv20.Amt, ticker.Max)
				sql = fmt.Sprintf(`
					UPDATE txos
					SET data = jsonb_set(data, '{bsv20}', data->'bsv20' || '{"status": 1, "reason":"%s", "amt":%d}')
					WHERE txid = $1 AND vout = $2`,
					reason,
					*bsv20.Amt,
				)
			} else {
				sql = `UPDATE txos
					SET data = jsonb_set(data, '{bsv20, status}', '1')
					WHERE txid = $1 AND vout = $2`
			}
			// fmt.Println("SQL:", sql)
			_, err = t.Exec(context.Background(),
				sql,
				bsv20.Txid,
				bsv20.Vout,
			)
			if err != nil {
				log.Panic(err)
			}

			_, err = t.Exec(context.Background(), `
				UPDATE bsv20_mints
				SET status=1
				WHERE txid = $1 AND vout=$2`,
				bsv20.Txid,
				bsv20.Vout,
			)
			if err != nil {
				log.Panic(err)
			}

			ticker.Supply += *bsv20.Amt
			_, err = t.Exec(context.Background(), `
				UPDATE bsv20
				SET supply=$3
				WHERE txid = $1 AND vout=$2`,
				ticker.Txid,
				ticker.Vout,
				ticker.Supply,
			)
			if err != nil {
				log.Panic(err)
			}

			err = t.Commit(context.Background())
			if err != nil {
				log.Panic(err)
			}
		}()
	}
}

func validateBsv20Transfers(height uint32) {
	rows, err := Db.Query(context.Background(), `
		SELECT txid
		FROM txos
		WHERE data @> '{"bsv20": {"op":"transfer", "status":0}}' AND height <= $1`,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	limiter := make(chan struct{}, THREADS)
	var wg sync.WaitGroup
	for rows.Next() {
		limiter <- struct{}{}
		wg.Add(1)
		txid := make([]byte, 32)
		err = rows.Scan(&txid)
		if err != nil {
			log.Panic(err)
		}
		go func() {
			defer func() {
				<-limiter
				wg.Done()
			}()
			ValidateTransfer(txid)
		}()
	}
}

func ValidateTransfer(txid []byte) {
	invalid := map[string]string{}
	pending := map[string]struct{}{}
	tokensIn := map[string]uint64{}

	// fmt.Printf("Validating transfer %x\n", txid)
	inRows, err := Db.Query(context.Background(), `
		SELECT COALESCE(data->'bsv20'->>'tick', data->'bsv20'->>'id') as tick, 
			CAST(data->'bsv20'->>'status' as INT) as status, 
			SUM(CAST(data->'bsv20'->>'amt' as numeric))
		FROM txos
		WHERE spend=$1 AND data IS NOT NULL
		GROUP BY tick, status`,
		txid,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer inRows.Close()

	for inRows.Next() {
		var tick sql.NullString
		var status Bsv20Status
		var amt uint64
		err = inRows.Scan(&tick, &status, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}

		if !tick.Valid {
			log.Panicf("%x - missing tick\n", txid)
		}
		switch status {
		case -1:
			invalid[tick.String] = "invalid"
		case 0:
			pending[tick.String] = struct{}{}
		case 1:
			tokensIn[tick.String] = amt
		}
	}

	outRows, err := Db.Query(context.Background(), `
		SELECT COALESCE(data->'bsv20'->>'tick', data->'bsv20'->>'id') as tick, 
			SUM(CAST(data->'bsv20'->>'amt' as numeric))
		FROM txos
		WHERE txid=$1 AND data IS NOT NULL
		GROUP BY tick`,
		txid,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer outRows.Close()

	for outRows.Next() {
		var tick string
		var amt uint64
		err = outRows.Scan(&tick, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}

		if balance, ok := tokensIn[tick]; ok {
			if amt > balance {
				invalid[tick] = "insufficient balance"
			}
			balance -= amt
			tokensIn[tick] = balance
		} else {
			invalid[tick] = "missing input"
		}
	}

	t, err := Db.Begin(context.Background())
	if err != nil {
		log.Panic(err)
	}
	defer t.Rollback(context.Background())

	for tick := range tokensIn {
		var sql string
		if _, ok := pending[tick]; ok {
			continue
		} else if reason, ok := invalid[tick]; ok {
			sql = fmt.Sprintf(`
				UPDATE txos
				SET data = jsonb_set(data, '{bsv20}', data->'bsv20' || '{"status": -1, "reason":"%s"}')
				WHERE txid = $1 AND (
					data @> '{"bsv20": {"tick":"%s"}}' OR
					data @> '{"bsv20": {"id":"%s"}}'
				)`,
				reason,
				tick,
				tick,
			)
		} else {
			sql = fmt.Sprintf(`
				UPDATE txos
				SET data = jsonb_set(data, '{bsv20,status}', '1')
				WHERE txid = $1 AND (
					data @> '{"bsv20": {"tick":"%s"}}' OR
					data @> '{"bsv20": {"id":"%s"}}'
				)`,
				tick,
				tick,
			)
		}
		_, err := t.Exec(context.Background(),
			sql,
			txid,
		)
		if err != nil {
			log.Panic(err)
		}
	}

	err = t.Commit(context.Background())
	if err != nil {
		log.Panic(err)
	}
}

func loadTicker(tick string) (ticker *Bsv20) {
	rows, err := Db.Query(context.Background(), `
		SELECT txid, vout, height, idx, tick, max, lim, supply
		FROM bsv20
		WHERE tick=$1 AND status=1`,
		tick,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		ticker = &Bsv20{}
		err = rows.Scan(&ticker.Txid, &ticker.Vout, &ticker.Height, &ticker.Idx, &ticker.Ticker, &ticker.Max, &ticker.Limit, &ticker.Supply)
		if err != nil {
			log.Panicln(tick, err)
		}
	}
	return
}

func setInvalid(t *pgx.Tx, txid []byte, vout uint32, reason string) {
	sql := fmt.Sprintf(`
		UPDATE txos
		SET data = jsonb_set(data, '{bsv20}', data->'bsv20' || '{"status": -1, "reason":"%s"}')
		WHERE txid = $1 AND vout = $2`,
		reason,
	)
	_, err := (*t).Exec(context.Background(), sql, txid, vout)
	if err != nil {
		log.Panic(err)
	}
}

func setValid(t *pgx.Tx, txid []byte, vout uint32, reason string) {
	var sql string
	if reason == "" {
		sql = `UPDATE txos
			SET data = jsonb_set(data, '{bsv20,status}', '1')
			WHERE txid = $1 AND vout = $2`
	} else {
		sql = fmt.Sprintf(`UPDATE txos
			SET data = jsonb_set(data, '{bsv20}', data->'bsv20' || '{"status": 1, "reason":"%s"}')
			WHERE txid = $1 AND vout = $2`,
			reason,
		)
	}
	_, err := (*t).Exec(context.Background(), sql, txid, vout)
	if err != nil {
		log.Panic(err)
	}
}
