package ordinals

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/shruggr/1sat-indexer/lib"
)

type Bsv20Status int

const (
	Invalid Bsv20Status = -1
	Pending Bsv20Status = 0
	Valid   Bsv20Status = 1
	MintFee uint64      = 100
)

type Bsv20 struct {
	Txid          []byte        `json:"-"`
	Vout          uint32        `json:"-"`
	Height        *uint32       `json:"-"`
	Idx           uint64        `json:"-"`
	Ticker        string        `json:"tick,omitempty"`
	Id            *lib.Outpoint `json:"id,omitempty"`
	Op            string        `json:"op"`
	Symbol        *string       `json:"sym,omitempty"`
	Max           uint64        `json:"-"`
	Limit         uint64        `json:"-"`
	Decimals      uint8         `json:"dec,omitempty"`
	Icon          *lib.Outpoint `json:"icon,omitempty"`
	Supply        uint64        `json:"-"`
	Amt           *uint64       `json:"amt"`
	Implied       bool          `json:"implied,omitempty"`
	Status        Bsv20Status   `json:"status"`
	Reason        *string       `json:"reason,omitempty"`
	PKHash        []byte        `json:"-"`
	Price         uint64        `json:"-"`
	PayOut        []byte        `json:"-"`
	Listing       bool          `json:"-"`
	PricePerToken float64       `json:"-"`
}

func ParseBsv20(ord *lib.File, height *uint32) (bsv20 *Bsv20, err error) {
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
		bsv20.Ticker = tick
	}

	if sym, ok := data["sym"]; ok {
		bsv20.Symbol = &sym
	}

	if id, ok := data["id"]; ok && len(id) >= 66 {
		bsv20.Id, err = lib.NewOutpointFromString(id)
		if err != nil {
			return nil, nil
		}
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
		if bsv20.Amt == nil {
			return nil, nil
		}
		bsv20.Ticker = ""
		bsv20.Status = Valid
		if icon, ok := data["icon"]; ok {
			bsv20.Icon, _ = lib.NewOutpointFromString(icon)
		}
	case "mint":
		if bsv20.Ticker == "" || bsv20.Amt == nil {
			return nil, nil
		}
	case "transfer":
		if bsv20.Amt == nil {
			return nil, nil
		}
		if bsv20.Ticker == "" && bsv20.Id == nil {
			return nil, nil
		}
	default:
		return nil, nil
	}

	return bsv20, nil
}

func (b *Bsv20) Save(t *lib.Txo) {
	if b.Op == "deploy" {
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20(txid, vout, height, idx, tick, max, lim, dec, status, reason)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT(txid, vout) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			t.Outpoint.Txid(),
			t.Outpoint.Vout(),
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
	}
	if b.Op == "deploy+mint" {
		b.Id = t.Outpoint
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20_v2(id, height, idx, sym, icon, amt, dec)
			VALUES($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT(id) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			t.Outpoint,
			t.Height,
			t.Idx,
			b.Symbol,
			b.Icon,
			b.Amt,
			b.Decimals,
		)
		if err != nil {
			log.Panic(err)
		}
	}
	if b.Op == "deploy+mint" || b.Op == "mint" || b.Op == "transfer" {
		// log.Println("BSV20 TXO:", b.Ticker, b.Id)
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20_txos(txid, vout, height, idx, id, tick, op, amt, pkhash, price, payout, listing, price_per_token, spend)
			SELECT txid, vout, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, spend
			FROM txos
			WHERE txid=$1 AND vout=$2
			ON CONFLICT(txid, vout) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx`,
			t.Outpoint.Txid(),
			t.Outpoint.Vout(),
			t.Height,
			t.Idx,
			b.Id,
			b.Ticker,
			b.Op,
			b.Amt,
			t.PKHash,
			b.Price,
			b.PayOut,
			b.Listing,
			b.PricePerToken,
		)

		// log.Println("BSV20 Listing:", t.Outpoint)
		// _, err := Db.Exec(context.Background(), `
		// 	INSERT INTO bsv20_txos(txid, vout, height, idx, price, payout, listing)
		// 	VALUES($1, $2, $3, $4, $5, $6, true)
		// 	ON CONFLICT(txid, vout) DO UPDATE SET
		// 		height=EXCLUDED.height,
		// 		idx=EXCLUDED.idx,
		// 		price=EXCLUDED.price,
		// 		payout=EXCLUDED.payout,
		// 		listing=EXCLUDED.listing`,
		// 	t.Outpoint.Txid(),
		// 	t.Outpoint.Vout(),
		// 	height,
		// 	t.Idx,
		// 	l.Price,
		// 	l.PayOut,
		// )
		// if err != nil {
		// 	log.Panicf("%x %v", t.Outpoint.Txid(), err)
		// }
		if err != nil {
			log.Panicf("%x %v", t.Outpoint.Txid(), err)
		}
	}
}

func ValidateBsv20(height uint32, tick string, concurrency int) {
	log.Println("Validating BSV20", height)
	// ValidateBsv20Deploy(height - 6)
	// // log.Println("Validating BSV20 mint", height)
	// validateBsv20Mint(height-6, concurrency)
	// // log.Println("Validating BSV20 transfers", height)
	// ValidateBsv20Transfers(height, concurrency)
	log.Println("Done validating BSV20", height)
}

func ValidateBsv20Deploy(height uint32) {
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

func ValidateBsv20DeployTick(height uint32, tick string) {
	rows, err := Db.Query(context.Background(), `
		SELECT txid, vout, height, idx, tick, max, lim, supply
		FROM bsv20
		WHERE tick=$1 AND status=0 AND height <= $2 AND height IS NOT NULL
		ORDER BY height ASC, idx ASC, vout ASC`,
		tick,
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

func ValidateBsv20Mints(height uint32, tick string) {
	// fmt.Println("[BSV20] Validating mint", tick, "to height", height)

	ticker := LoadTicker(tick)

	rows, err := Db.Query(context.Background(), `
		SELECT txid, vout, height, idx, tick, amt
		FROM bsv20_txos
		WHERE op='mint' AND tick=$1 AND status=0 AND height <= $2 AND height > 0
		ORDER BY height ASC, idx ASC, vout ASC`,
		tick,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	mints := make([]*Bsv20, 0, 1000)
	for rows.Next() {
		bsv20 := &Bsv20{}
		err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Ticker, &bsv20.Amt)
		if err != nil {
			log.Panic(err)
		}
		mints = append(mints, bsv20)
	}

	for _, bsv20 := range mints {
		// fmt.Printf("Validating BSV20 mint %s %x\n", tick, bsv20.Txid)

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
				_, err = Db.Exec(context.Background(), `
					UPDATE bsv20_txos
					SET status=-1, reason=$3
					WHERE txid=$1 AND vout=$2`,
					bsv20.Txid,
					bsv20.Vout,
					reason,
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

			var reason string
			if ticker.Max-ticker.Supply < *bsv20.Amt {
				*bsv20.Amt = ticker.Max - ticker.Supply
				reason = fmt.Sprintf("supply %d + amt %d > max %d", ticker.Supply, *bsv20.Amt, ticker.Max)

				_, err := t.Exec(context.Background(), `
					UPDATE bsv20_txos
					SET status=1, amt=$3, reason=$4
					WHERE txid = $1 AND vout=$2`,
					bsv20.Txid,
					bsv20.Vout,
					*bsv20.Amt,
					reason,
				)
				if err != nil {
					log.Panic(err)
				}
			} else {
				_, err := t.Exec(context.Background(), `
					UPDATE bsv20_txos
					SET status=1
					WHERE txid = $1 AND vout=$2`,
					bsv20.Txid,
					bsv20.Vout,
				)
				if err != nil {
					log.Panic(err)
				}
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

func ValidateBsv20Transfers(tick string, height uint32, concurrency int) {
	// fmt.Printf("[BSV20] Validating transfers. Tick %s Height %d\n", tick, height)
	rows, err := Db.Query(context.Background(), `
		SELECT txid
		FROM (
			SELECT txid, MIN(height) as height
			FROM bsv20_txos
			WHERE status=0 AND op='transfer' AND tick=$1 AND height <= $2
			GROUP by txid
		) t
		ORDER BY height ASC`,
		tick,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	limiter := make(chan struct{}, concurrency)
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
			ValidateTransfer(txid, true)
			// time.Sleep(10 * time.Second)
		}()
	}
	wg.Wait()
}

func ValidateBsv20V2Transfers(id *lib.Outpoint, height uint32, concurrency int) {
	// fmt.Printf("[BSV20] Validating transfers. Id: %s Height %d\n", id, height)
	rows, err := Db.Query(context.Background(), `
		SELECT txid
		FROM (
			SELECT txid, MIN(height) as height
			FROM bsv20_txos
			WHERE status=0 AND op='transfer' AND id=$1 AND height <= $2
			GROUP by txid
		) t
		ORDER BY height ASC`,
		id,
		height,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	limiter := make(chan struct{}, concurrency)
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
			ValidateTransfer(txid, true)
			// time.Sleep(10 * time.Second)
		}()
	}
	wg.Wait()
}

func ValidateTransfer(txid []byte, mined bool) {
	invalid := map[string]string{}
	tokensIn := map[string]uint64{}
	tickTokens := map[string][]uint32{}

	inRows, err := Db.Query(context.Background(), `
		SELECT txid, vout, id, tick, status, amt
		FROM bsv20_txos
		WHERE spend=$1`,
		txid,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer inRows.Close()

	for inRows.Next() {
		var inTxid []byte
		var vout uint32
		var id []byte
		var tick string
		var amt uint64
		var status int
		err = inRows.Scan(&inTxid, &vout, &id, &tick, &status, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}

		if len(id) > 0 {
			tokenId := lib.Outpoint(id)
			tick = tokenId.String()
		} else if tick == "" {
			log.Panicf("%x - missing tick\n", txid)
		}

		switch status {
		case -1:
			invalid[tick] = "invalid inputs"
		case 0:
			return
		case 1:
			if _, ok := tokensIn[tick]; !ok {
				tokensIn[tick] = amt
			} else {
				tokensIn[tick] += amt
			}
		}
	}
	inRows.Close()

	outRows, err := Db.Query(context.Background(), `
		SELECT vout, id, tick, status, amt
		FROM bsv20_txos
		WHERE txid=$1`,
		txid,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer outRows.Close()

	for outRows.Next() {
		var vout uint32
		var id []byte
		var tick string
		var amt uint64
		var status int
		err = outRows.Scan(&vout, &id, &tick, &status, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}
		if len(id) > 0 {
			tokenId := lib.Outpoint(id)
			tick = tokenId.String()
		} else if tick == "" {
			log.Panicf("%x - missing tick\n", txid)
		}

		if _, ok := tickTokens[tick]; !ok {
			tickTokens[tick] = []uint32{}
		}
		tickTokens[tick] = append(tickTokens[tick], vout)
		if balance, ok := tokensIn[tick]; ok {
			if amt > balance {
				invalid[tick] = fmt.Sprintf("insufficient balance %d < %d", balance, amt)
			} else {
				balance -= amt
				tokensIn[tick] = balance
			}
		} else {
			if _, ok := invalid[tick]; !ok {
				if !mined {
					return
				} else {
					invalid[tick] = "missing inputs"
				}
			}
		}
	}

	t, err := Db.Begin(context.Background())
	if err != nil {
		log.Panic(err)
	}
	defer t.Rollback(context.Background())

	for tick := range tickTokens {
		var sql string
		vouts := tickTokens[tick]
		if reason, ok := invalid[tick]; ok {
			_, err := t.Exec(context.Background(), `
				UPDATE bsv20_txos
				SET status=-1, reason=$3
				WHERE txid=$1 AND vout=ANY($2)`,
				txid,
				vouts,
				reason,
			)
			if err != nil {
				log.Panicf("%x %s %v\n", txid, sql, err)
			}
		} else {
			_, err := t.Exec(context.Background(), `
				UPDATE bsv20_txos
				SET status=1
				WHERE txid=$1 AND vout=ANY ($2)`,
				txid,
				vouts,
			)
			if err != nil {
				log.Panicf("%x %s %v\n", txid, sql, err)
			}
		}
	}

	err = t.Commit(context.Background())
	if err != nil {
		log.Panic(err)
	}
}

func LoadTicker(tick string) (ticker *Bsv20) {
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

func LoadTokenById(id *lib.Outpoint) (token *Bsv20) {
	rows, err := Db.Query(context.Background(), `
		SELECT id, height, idx, sym, icon, dec, amt
		FROM bsv20_v2
		WHERE tick=$1`,
		id,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		token = &Bsv20{}
		err = rows.Scan(&token.Id, &token.Height, &token.Idx, &token.Symbol, &token.Icon, &token.Decimals, &token.Amt)
		if err != nil {
			log.Panicln(id, err)
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
