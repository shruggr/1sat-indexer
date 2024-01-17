package ordinals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/libsv/go-bk/bip32"
	"github.com/libsv/go-bk/crypto"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordlock"
)

type Bsv20Status int

var hdKey, _ = bip32.NewKeyFromString("xpub661MyMwAqRbcF221R74MPqdipLsgUevAAX4hZP2rywyEeShpbe3v2r9ciAvSGT6FB22TEmFLdUyeEDJL4ekG8s9H5WXbzDQPr6eW1zEYYy9")

const (
	Invalid Bsv20Status = -1
	Pending Bsv20Status = 0
	Valid   Bsv20Status = 1
	MintFee uint64      = 100
)

const BSV20V1_OP_COST = 1000
const BSV20V2_OP_COST = 1000

var ctx = context.Background()

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
	FundPath      string        `json:"-"`
	FundPKHash    []byte        `json:"-"`
	FundBalance   int           `json:"-"`
}

func IndexBsv20(ctx *lib.IndexContext) (token *Bsv20) {
	for _, txo := range ctx.Txos {
		if bsv20, ok := txo.Data["bsv20"].(*Bsv20); ok {
			if bsv20.Ticker != "" {
				token = LoadTicker(bsv20.Ticker)
			} else {
				token = LoadTokenById(bsv20.Id)
			}
			list := ordlock.ParseScript(txo)

			if list != nil {
				txo.PKHash = list.PKHash
				bsv20.PKHash = list.PKHash
				bsv20.Price = list.Price
				bsv20.PayOut = list.PayOut
				bsv20.Listing = true

				var decimals uint8
				if token != nil {
					decimals = token.Decimals
				}
				bsv20.PricePerToken = float64(bsv20.Price) / float64(*bsv20.Amt*(10^uint64(decimals)))
			}
			bsv20.Save(txo)
		}
	}
	return
}

func ParseBsv20Inscription(ord *lib.File, txo *lib.Txo) (bsv20 *Bsv20, err error) {
	mime := strings.ToLower(ord.Type)
	if !strings.HasPrefix(mime, "application/bsv-20") &&
		!(txo.Height != nil && *txo.Height < 793000 && strings.HasPrefix(mime, "text/plain")) {
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
		hash := sha256.Sum256([]byte(bsv20.Ticker))
		path := fmt.Sprintf("21/%d/%d", binary.BigEndian.Uint32(hash[:8])>>1, binary.BigEndian.Uint32(hash[24:])>>1)
		ek, err := hdKey.DeriveChildFromPath(path)
		if err != nil {
			log.Panic(err)
		}
		pubKey, err := ek.ECPubKey()
		if err != nil {
			log.Panic(err)
		}
		bsv20.FundPath = path
		bsv20.FundPKHash = crypto.Hash160(pubKey.SerialiseCompressed())
	case "deploy+mint":
		if bsv20.Amt == nil {
			return nil, nil
		}
		if sym, ok := data["sym"]; ok {
			bsv20.Symbol = &sym
		}
		bsv20.Ticker = ""
		bsv20.Status = Valid
		if icon, ok := data["icon"]; ok {
			bsv20.Icon, _ = lib.NewOutpointFromString(icon)
		}
		bsv20.Id = txo.Outpoint
		hash := sha256.Sum256(*bsv20.Id)
		path := fmt.Sprintf("21/%d/%d", binary.BigEndian.Uint32(hash[:8])>>1, binary.BigEndian.Uint32(hash[24:])>>1)
		ek, err := hdKey.DeriveChildFromPath(path)
		if err != nil {
			log.Panic(err)
		}
		pubKey, err := ek.ECPubKey()
		if err != nil {
			log.Panic(err)
		}
		bsv20.FundPath = path
		bsv20.FundPKHash = crypto.Hash160(pubKey.SerialiseCompressed())
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
			INSERT INTO bsv20(txid, vout, height, idx, tick, max, lim, dec, status, reason, fund_path, fund_pkhash)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT(txid, vout) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				fund_path=EXCLUDED.fund_path,
				fund_pkhash=EXCLUDED.fund_pkhash`,
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
			b.FundPath,
			b.FundPKHash,
		)
		if err != nil {
			log.Panic(err)
		}
	}
	if b.Op == "deploy+mint" {
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20_v2(id, height, idx, sym, icon, amt, dec, fund_path, fund_pkhash)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT(id) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				sym=EXCLUDED.sym,
				icon=EXCLUDED.icon,
				fund_path=EXCLUDED.fund_path,
				fund_pkhash=EXCLUDED.fund_pkhash`,
			t.Outpoint,
			t.Height,
			t.Idx,
			b.Symbol,
			b.Icon,
			b.Amt,
			b.Decimals,
			b.FundPath,
			b.FundPKHash,
		)
		if err != nil {
			log.Panic(err)
		}
	}
	if b.Op == "deploy+mint" || b.Op == "mint" || b.Op == "transfer" {
		// log.Println("BSV20 TXO:", b.Ticker, b.Id)

		// log.Printf("BSV20 TXO: %s %d\n", b.Id, len(t.Script))
		_, err := Db.Exec(context.Background(), `
			INSERT INTO bsv20_txos(txid, vout, height, idx, id, tick, op, amt, pkhash, price, payout, listing, price_per_token, script, status, spend)
			SELECT txid, vout, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, spend
			FROM txos
			WHERE txid=$1 AND vout=$2
			ON CONFLICT(txid, vout) DO UPDATE SET
				height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				pkhash=EXCLUDED.pkhash,
				status=EXCLUDED.status,
				script=EXCLUDED.script,
				price=EXCLUDED.price,
				payout=EXCLUDED.payout,
				listing=EXCLUDED.listing,
				price_per_token=EXCLUDED.price_per_token`,
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
			t.Script,
			b.Status,
		)

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

func ValidateBsv20MintsSubs(height uint32, topic string) {
	rows, err := Db.Query(context.Background(), `
		SELECT DISTINCT b.tick, b.fund_balance
		FROM bsv20_subs s
		JOIN bsv20 b ON b.tick=s.tick AND b.fund_balance>=$2
		WHERE s.topic=$1 AND b.status=1`,
		topic,
		BSV20V1_OP_COST,
	)

	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var tick string
		var balance uint
		err = rows.Scan(&tick, &balance)
		if err != nil {
			log.Panic(err)
		}

		ValidateBsv20V1Mints(height, tick, balance/BSV20V1_OP_COST)
	}
}

func ValidateBsv20V1Mints(height uint32, tick string, limit uint) {
	fmt.Println("Validating Mints:", tick, height)
	ticker := LoadTicker(tick)
	rows, err := Db.Query(context.Background(), `
		SELECT txid, vout, height, idx, tick, amt
		FROM bsv20_txos
		WHERE op = 'mint' AND tick=$1 AND status=0 AND height<=$2 AND height > 0
		ORDER BY height ASC, idx ASC, vout ASC
		LIMIT $3`,
		tick,
		height,
		limit,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		bsv20 := &Bsv20{}
		err = rows.Scan(&bsv20.Txid, &bsv20.Vout, &bsv20.Height, &bsv20.Idx, &bsv20.Ticker, &bsv20.Amt)
		if err != nil {
			log.Panic(err)
		}

		var reason string
		if ticker == nil || *ticker.Height > *bsv20.Height || (*ticker.Height == *bsv20.Height && ticker.Idx > bsv20.Idx) {
			reason = fmt.Sprintf("invalid ticker %s as of %d %d", tick, *bsv20.Height, bsv20.Idx)
		} else if ticker.Supply >= ticker.Max {
			reason = fmt.Sprintf("supply %d >= max %d", ticker.Supply, ticker.Max)
		} else if ticker.Limit > 0 && *bsv20.Amt > ticker.Limit {
			reason = fmt.Sprintf("amt %d > limit %d", *bsv20.Amt, ticker.Limit)
		}
		func() {
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
				return
			}

			t, err := Db.Begin(context.Background())
			if err != nil {
				log.Panic(err)
			}
			defer t.Rollback(context.Background())

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

func ValidatePaidBsv20V1Transfers(concurrency int, height uint32) {
	log.Println("Validating Paid V1 Xfers", height)
	rows, err := Db.Query(context.Background(), `
		SELECT b.tick, b.fund_balance
		FROM bsv20 b
		JOIN bsv20_subs s ON s.tick=b.tick
		WHERE b.status=1 AND b.fund_balance >= $1 AND EXISTS (
			SELECT 1 FROM bsv20_txos WHERE tick=b.tick AND op='transfer' AND status=0
		)`,
		BSV20V2_OP_COST,
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
		var tick string
		var balance int
		err = rows.Scan(&tick, &balance)
		if err != nil {
			log.Panic(err)
		}
		go func(tick string, balance int) {
			fmt.Println("VALIDATING", tick, balance)
			defer func() {
				<-limiter
				wg.Done()
			}()

			rows, err := Db.Query(context.Background(), ` 
				SELECT txid
				FROM bsv20_txos
				WHERE status=0 AND op='transfer' AND tick=$1 AND height <= $2
				ORDER BY height, idx, vout
				LIMIT $3`,
				tick,
				height,
				balance/BSV20V1_OP_COST,
			)
			if err != nil {
				log.Panicln(err)
			}
			defer rows.Close()
			var prevTxid []byte
			for rows.Next() {
				var txid []byte
				err = rows.Scan(&txid)
				if err != nil {
					log.Panicln(err)
				}
				if bytes.Equal(prevTxid, txid) {
					continue
				}
				prevTxid = txid
				ValidateV1Transfer(txid, tick, true)
			}
		}(tick, balance)
	}
	wg.Wait()
	UpdateBsv20V1Funding()
}

func ValidateV1Transfer(txid []byte, tick string, mined bool) {
	// log.Printf("Validating %x %s\n", txid, tick)
	balance, err := Rdb.Get(context.Background(), "funds:"+tick).Int64()
	if err != nil {
		panic(err)
	}
	if balance < BSV20V1_OP_COST {
		log.Printf("Inadequate funding %x %s\n", txid, tick)
		return
	}

	inRows, err := Db.Query(context.Background(), `
		SELECT txid, vout, status, amt
		FROM bsv20_txos
		WHERE spend=$1 AND tick=$2`,
		txid,
		tick,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer inRows.Close()

	var reason string
	var tokensIn uint64
	var tokenOuts []uint32
	for inRows.Next() {
		var inTxid []byte
		var vout uint32
		var amt uint64
		var inStatus int
		err = inRows.Scan(&inTxid, &vout, &inStatus, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}

		switch inStatus {
		case -1:
			reason = "invalid input"
		case 0:
			fmt.Printf("inputs pending %s %x\n", tick, txid)
			return
		case 1:
			tokensIn += amt
		}
	}
	inRows.Close()

	sql := `SELECT vout, status, amt
		FROM bsv20_txos
		WHERE txid=$1 AND tick=$2 AND op='transfer'`
	outRows, err := Db.Query(context.Background(),
		sql,
		txid,
		tick,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer outRows.Close()

	for outRows.Next() {
		var vout uint32
		var amt uint64
		var status int
		err = outRows.Scan(&vout, &status, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}
		tokenOuts = append(tokenOuts, vout)
		if amt > tokensIn {
			if reason == "" {
				reason = fmt.Sprintf("insufficient balance %d < %d", tokensIn, amt)
			}
			if !mined {
				fmt.Printf("%s %s - %x\n", tick, reason, txid)
				return
			}
		} else {
			tokensIn -= amt
		}
	}

	if reason != "" {
		log.Printf("Transfer Invalid: %x %s %s\n", txid, tick, reason)
		sql := `UPDATE bsv20_txos
			SET status=-1, reason=$3
			WHERE txid=$1 AND vout=ANY($2)`
		_, err := Db.Exec(context.Background(), sql,
			txid,
			tokenOuts,
			reason,
		)
		if err != nil {
			log.Panicf("%x %v\n", txid, err)
		}
	} else {
		log.Printf("Transfer Valid: %x %s\n", txid, tick)
		_, err := Db.Exec(context.Background(), `
				UPDATE bsv20_txos
				SET status=1
				WHERE txid=$1 AND vout=ANY ($2)`,
			txid,
			tokenOuts,
		)
		if err != nil {
			log.Panicf("%x %v\n", txid, err)
		}
	}
	Rdb.IncrBy(context.Background(), "funds:"+tick, int64(len(tokenOuts)*BSV20V1_OP_COST*-1))

}

func ValidatePaidBsv20V2Transfers(concurrency int, height uint32) {
	log.Println("Validating Paid V2 Xfers")
	rows, err := Db.Query(context.Background(), `
		SELECT b.id, b.fund_balance
		FROM bsv20_v2 b
		WHERE b.fund_balance >= $1 AND EXISTS (
			SELECT 1 FROM bsv20_txos WHERE id=b.id AND op='transfer' AND status=0
		)`,
		BSV20V2_OP_COST,
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
		id := &lib.Outpoint{}
		var balance int
		err = rows.Scan(&id, &balance)
		if err != nil {
			log.Panic(err)
		}
		go func(id *lib.Outpoint, balance int) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			rows, err := Db.Query(context.Background(), `
				SELECT txid
				FROM bsv20_txos
				WHERE status=0 AND op='transfer' AND id=$1 AND height <= $2
				ORDER BY height, idx, vout
				LIMIT $3`,
				id,
				height,
				balance/BSV20V2_OP_COST,
			)
			if err != nil {
				log.Panicln(err)
			}
			defer rows.Close()
			var prevTxid []byte
			for rows.Next() {
				var txid []byte
				err = rows.Scan(&txid)
				if err != nil {
					log.Panicln(err)
				}
				if bytes.Equal(prevTxid, txid) {
					continue
				}
				prevTxid = txid
				ValidateV2Transfer(txid, id, true)
			}
		}(id, balance)
	}
	wg.Wait()
}

func ValidateBsvPaid20V2Transfer(txid []byte, id *lib.Outpoint, mined bool) {
	rows, err := Db.Query(context.Background(), `
		SELECT 1
		FROM bsv20_v2
		WHERE txid=$1 AND fund_balance>=$2`,
		txid,
		BSV20V2_OP_COST,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()

	if rows.Next() {
		ValidateV2Transfer(txid, id, true)
	}
}

func ValidateBsv20V2Transfers(id *lib.Outpoint, height uint32, concurrency int) {
	// fmt.Printf("[BSV20] Validating transfers. Id: %s Height %d\n", id, height)
	rows, err := Db.Query(context.Background(), `
		SELECT txid
		FROM (
			SELECT txid, MIN(height) as height, MIN(idx) as idx
			FROM bsv20_txos
			WHERE status=0 AND op='transfer' AND id=$1 AND height<=$2
			GROUP by txid
		) t
		ORDER BY height ASC, idx ASC`,
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

func ValidateV2Transfer(txid []byte, id *lib.Outpoint, mined bool) {
	// log.Printf("Validating V2 Transfer %x %s\n", txid, id.String())
	balance, err := Rdb.Get(context.Background(), "funds:"+id.String()).Int64()
	if err != nil {
		panic(err)
	}
	if balance < BSV20V1_OP_COST {
		log.Printf("Inadequate funding %x %s\n", txid, id.String())
		return
	}

	inRows, err := Db.Query(context.Background(), `
		SELECT txid, vout, status, amt
		FROM bsv20_txos
		WHERE spend=$1 AND id=$2`,
		txid,
		id,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer inRows.Close()

	var reason string
	var tokensIn uint64
	var tokenOuts []uint32
	for inRows.Next() {
		var inTxid []byte
		var vout uint32
		var amt uint64
		var inStatus int
		err = inRows.Scan(&inTxid, &vout, &inStatus, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}

		switch inStatus {
		case -1:
			reason = "invalid input"
		case 0:
			fmt.Printf("inputs pending %s %x\n", id.String(), txid)
			return
		case 1:
			tokensIn += amt
		}
	}
	inRows.Close()

	outRows, err := Db.Query(context.Background(), `
		SELECT vout, status, amt
		FROM bsv20_txos
		WHERE txid=$1 AND id=$2`,
		txid,
		id,
	)
	if err != nil {
		log.Panicf("%x - %v\n", txid, err)
	}
	defer outRows.Close()

	for outRows.Next() {
		var vout uint32
		var amt uint64
		var status int
		err = outRows.Scan(&vout, &status, &amt)
		if err != nil {
			log.Panicf("%x - %v\n", txid, err)
		}
		tokenOuts = append(tokenOuts, vout)
		if reason != "" {
			continue
		}
		if amt > tokensIn {
			reason = fmt.Sprintf("insufficient balance %d < %d", tokensIn, amt)
			if !mined {
				fmt.Printf("%s %s - %x\n", id.String(), reason, txid)
				return
			}
		} else {
			tokensIn -= amt
		}
	}

	t, err := Db.Begin(context.Background())
	if err != nil {
		log.Panic(err)
	}
	defer t.Rollback(context.Background())

	if reason != "" {
		log.Printf("Transfer Invalid: %x %s %s\n", txid, id.String(), reason)
		_, err := t.Exec(context.Background(), `
				UPDATE bsv20_txos
				SET status=-1, reason=$3
				WHERE txid=$1 AND vout=ANY($2)`,
			txid,
			tokenOuts,
			reason,
		)
		if err != nil {
			log.Panicf("%x %v\n", txid, err)
		}
	} else {
		log.Printf("Transfer Valid: %x %s\n", txid, id.String())
		_, err := t.Exec(context.Background(), `
				UPDATE bsv20_txos
				SET status=1
				WHERE txid=$1 AND vout=ANY ($2)`,
			txid,
			tokenOuts,
		)
		if err != nil {
			log.Panicf("%x %v\n", txid, err)
		}
		if id != nil {
			_, err := t.Exec(context.Background(), `
					UPDATE bsv20_v2
					SET fund_used=fund_used+$2
					WHERE id=$1`,
				id,
				BSV20V2_OP_COST*len(tokenOuts),
			)
			if err != nil {
				log.Panicf("%x %v\n", txid, err)
			}
		}
	}

	err = t.Commit(context.Background())
	if err != nil {
		log.Panic(err)
	}
}

func ValidateTransfer(txid []byte, mined bool) {
	log.Printf("Validating Transfer %x\n", txid)
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
		id, _ := lib.NewOutpointFromString(tick)

		if reason, ok := invalid[tick]; ok {
			log.Printf("Transfer Invalid: %x %s\n", txid, reason)
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
			log.Printf("Transfer Valid: %x\n", txid)
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
			if id != nil {
				_, err := t.Exec(context.Background(), `
					UPDATE bsv20_v2
					SET fund_used=fund_used+$2
					WHERE id=$1`,
					id,
					BSV20V2_OP_COST,
				)
				if err != nil {
					log.Panicf("%x %s %v\n", txid, sql, err)
				}
			} else {
				_, err := t.Exec(context.Background(), `
					UPDATE bsv20
					SET fund_used=fund_used+$2
					WHERE tick=$1 AND status=1`,
					tick,
					BSV20V2_OP_COST,
				)
				if err != nil {
					log.Panicf("%x %s %v\n", txid, sql, err)
				}
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
		SELECT txid, vout, height, idx, tick, max, lim, supply, fund_balance
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
		err = rows.Scan(&ticker.Txid, &ticker.Vout, &ticker.Height, &ticker.Idx, &ticker.Ticker, &ticker.Max, &ticker.Limit, &ticker.Supply, &ticker.FundBalance)
		if err != nil {
			log.Panicln(tick, err)
		}
	}
	return
}

func LoadTokenById(id *lib.Outpoint) (token *Bsv20) {
	rows, err := Db.Query(context.Background(), `
		SELECT id, height, idx, sym, icon, dec, amt, fund_balance
		FROM bsv20_v2
		WHERE id=$1`,
		id,
	)
	if err != nil {
		log.Panic(err)
	}
	defer rows.Close()
	if rows.Next() {
		token = &Bsv20{}
		err = rows.Scan(&token.Id, &token.Height, &token.Idx, &token.Symbol, &token.Icon, &token.Decimals, &token.Amt, &token.FundBalance)
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

type V1TokenFunds struct {
	Tick   string
	Total  int64
	PKHash []byte
	Used   int64
}

func (t *V1TokenFunds) Save() {
	_, err := Db.Exec(context.Background(), `
		UPDATE bsv20
		SET fund_total=$2, fund_used=$3
		WHERE tick=$1`,
		t.Tick,
		t.Total,
		t.Used,
	)
	if err != nil {
		panic(err)
	}
	Rdb.Set(context.Background(), "funds:"+t.Tick, t.Balance(), 0)
}

func (t *V1TokenFunds) Balance() int64 {
	return t.Total - t.Used
}

type V2TokenFunds struct {
	Id     *lib.Outpoint
	Total  int64
	PKHash []byte
	Used   int64
}

func (t *V2TokenFunds) Save() {
	_, err := Db.Exec(context.Background(), `
		UPDATE bsv20_v2
		SET fund_total=$2, fund_used=$3
		WHERE id=$1`,
		t.Id,
		t.Total,
		t.Used,
	)
	if err != nil {
		panic(err)
	}
	Rdb.Set(context.Background(), "funds:"+t.Id.String(), t.Balance(), 0)
}

func (t *V2TokenFunds) Balance() int64 {
	return t.Total - t.Used
}

func UpdateBsv20V1Funding() map[string]*V1TokenFunds {
	tickFunds := map[string]*V1TokenFunds{}
	rows, err := Db.Query(context.Background(), `
		SELECT b.tick, b.fund_pkhash 
		FROM bsv20 b
		JOIN bsv20_subs s ON s.tick=b.tick
		WHERE b.status=1
	`)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		tick := &V1TokenFunds{}
		err = rows.Scan(&tick.Tick, &tick.PKHash)
		if err != nil {
			log.Panicln(err)
		}
		tickFunds[tick.Tick] = tick
	}
	rows.Close()

	rows, err = Db.Query(context.Background(), `
		SELECT b.tick, SUM(satoshis) as total 
		FROM txos t
		JOIN bsv20 b ON b.fund_pkhash=t.pkhash AND b.status=1
		JOIN bsv20_subs s ON s.tick=b.tick
		GROUP BY b.tick	
	`)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tick string
		var total int64
		err = rows.Scan(&tick, &total)
		if err != nil {
			log.Panicln(err)
		}
		funds := tickFunds[tick]
		funds.Total = total
	}

	rows, err = Db.Query(context.Background(), `
		SELECT t.tick, COUNT(1) as ops 
		FROM bsv20_txos t
		JOIN bsv20_subs s ON s.tick=t.tick
		WHERE t.status IN (-1, 1)
		GROUP BY t.tick`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tick string
		var ops int
		err = rows.Scan(&tick, &ops)
		if err != nil {
			log.Panicln(err)
		}
		funds := tickFunds[tick]
		funds.Used = int64(ops * BSV20V1_OP_COST)
	}

	for _, funds := range tickFunds {
		funds.Save()
	}
	return tickFunds
}

func UpdateBsv20V2Funding() map[string]*V2TokenFunds {
	tickFunds := map[string]*V2TokenFunds{}
	rows, err := Db.Query(context.Background(), `
		SELECT id, fund_pkhash 
		FROM bsv20_v2`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		tick := &V2TokenFunds{}
		err = rows.Scan(&tick.Id, &tick.PKHash)
		if err != nil {
			log.Panicln(err)
		}
		tickFunds[tick.Id.String()] = tick
	}
	rows.Close()

	rows, err = Db.Query(context.Background(), `
		SELECT b.id, SUM(satoshis) as total 
		FROM txos t
		JOIN bsv20_v2 b ON b.fund_pkhash=t.pkhash
		GROUP BY b.id	
	`)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id *lib.Outpoint
		var total int64
		err = rows.Scan(&id, &total)
		if err != nil {
			log.Panicln(err)
		}
		funds := tickFunds[id.String()]
		funds.Total = total
	}

	rows, err = Db.Query(context.Background(), `
		SELECT t.id, COUNT(1) as ops 
		FROM bsv20_txos t
		WHERE t.id != '\x' AND t.status IN (-1, 1)
		GROUP BY t.id`,
	)
	if err != nil {
		log.Panicln(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id *lib.Outpoint
		var ops int
		err = rows.Scan(&id, &ops)
		if err != nil {
			log.Panicln(err)
		}
		funds := tickFunds[id.String()]
		funds.Used = int64(ops * BSV20V2_OP_COST)
	}

	for _, funds := range tickFunds {
		funds.Save()
	}
	return tickFunds
}

// func UpdateBsv20V2FundingForId(id *lib.Outpoint) {
// 	_, err := Db.Exec(ctx, `UPDATE bsv20_v2 v
// 		SET fund_total=s.total
// 		FROM (
// 			SELECT pkhash, SUM(satoshis) as total
// 			FROM txos
// 			GROUP BY pkhash
// 		) s
// 		WHERE s.pkhash = v.fund_pkhash AND v.id=$1`,
// 		id,
// 	)
// 	if err != nil {
// 		log.Panicln(err)
// 	}
// }
