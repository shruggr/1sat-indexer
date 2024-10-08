package ordlock

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
	"github.com/shruggr/1sat-indexer/lib"
)

type Sale struct {
	Spend    lib.ByteString `json:"spend"`
	Outpoint *lib.Outpoint  `json:"outpoint"`
	Price    uint64         `json:"price,omitempty"`
	Tick     *string        `json:"tick,omitempty"`
	Id       *lib.Outpoint  `json:"id,omitempty"`
	Amt      uint64         `json:"amt,omitempty"`
	Seller   *lib.PKHash    `json:"seller,omitempty"`
	Buyer    *lib.PKHash    `json:"buyer,omitempty"`
	PricePer float64        `json:"pricePer,omitempty"`
}

type Listing struct {
	PKHash *lib.PKHash `json:"-"`
	Price  uint64      `json:"price"`
	PayOut []byte      `json:"payout"`
}

type ListingEvent struct {
	Outpoint *lib.Outpoint `json:"outpoint"`
	PKHash   *lib.PKHash   `json:"owner"`
	Price    uint64        `json:"price"`
}

var OrdLockSuffix, _ = hex.DecodeString("615179547a75537a537a537a0079537a75527a527a7575615579008763567901c161517957795779210ac407f0e4bd44bfc207355a778b046225a7068fc59ee7eda43ad905aadbffc800206c266b30e6a1319c66dc401e5bd6b432ba49688eecd118297041da8074ce081059795679615679aa0079610079517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01007e81517a75615779567956795679567961537956795479577995939521414136d08c5ed2bf3ba048afe6dcaebafeffffffffffffffffffffffffffffff00517951796151795179970079009f63007952799367007968517a75517a75517a7561527a75517a517951795296a0630079527994527a75517a6853798277527982775379012080517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01205279947f7754537993527993013051797e527e54797e58797e527e53797e52797e57797e0079517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a756100795779ac517a75517a75517a75517a75517a75517a75517a75517a75517a7561517a75517a756169587951797e58797eaa577961007982775179517958947f7551790128947f77517a75517a75618777777777777777777767557951876351795779a9876957795779ac777777777777777767006868")

func ParseOrdinalLocks(ctx *lib.IndexContext) {
	for _, txo := range ctx.Txos {
		list := ParseScript(txo)
		if list != nil {
			txo.AddData("list", list)
			txo.PKHash = list.PKHash
		}
	}
}

func ProcessSpends(ctx *lib.IndexContext) {
	if !bytes.Contains(*ctx.Tx.Inputs[0].UnlockingScript, OrdLockSuffix) {
		return
	}

	buyer := ctx.Txos[0].PKHash
	if _, ok := ctx.Txos[0].Data["bsv20"]; ok {
		rows, err := lib.Db.Query(context.Background(), `
			UPDATE bsv20_txos
			SET sale=true, spend_height=$2, spend_idx=$3, buyer=$4
			WHERE spend=$1
			RETURNING txid, vout, tick, id, price, amt, pkhash, price_per_token`,
			ctx.Txid,
			ctx.Height,
			ctx.Idx,
			buyer,
		)
		if err != nil {
			log.Panicln(err)
		}
		defer rows.Close()
		for rows.Next() {
			var txid []byte
			var vout uint32
			bsv20Sale := &Sale{
				Spend: ctx.Txid,
				Buyer: buyer,
			}
			err := rows.Scan(&txid, &vout, &bsv20Sale.Tick, &bsv20Sale.Id, &bsv20Sale.Price, &bsv20Sale.Amt, &bsv20Sale.Seller, &bsv20Sale.PricePer)
			if err != nil {
				log.Panicln(err)
			}
			bsv20Sale.Outpoint = lib.NewOutpoint(txid, vout)
			out, err := json.Marshal(bsv20Sale)
			if err != nil {
				log.Panicln(err)
			}
			log.Println("PUBLISHING BSV20 SALE", string(out))
			lib.PublishEvent(context.Background(), "bsv20Sale", string(out))
		}
	} else {
		var txid []byte
		var vout uint32
		ordSale := &Sale{
			Spend: ctx.Txid,
			Amt:   1,
			Buyer: buyer,
			// Seller: &lib.PKHash{},
		}
		log.Println("ORDLOCK", hex.EncodeToString(ctx.Txid))
		rows, err := lib.Db.Query(context.Background(), `
			UPDATE listings
			SET sale=true, spend_height=$2, spend_idx=$3, buyer=$4
			WHERE spend=$1
			RETURNING txid, vout, price, pkhash`,
			ctx.Txid,
			ctx.Height,
			ctx.Idx,
			buyer,
		)
		if err != nil {
			log.Panicln(err)
		}
		defer rows.Close()
		if rows.Next() {
			if err := rows.Scan(&txid, &vout, &ordSale.Price, &ordSale.Seller); err != nil {
				log.Panicln(err)
			}
			ordSale.PricePer = float64(ordSale.Price)
			ordSale.Outpoint = lib.NewOutpoint(txid, vout)
			if out, err := json.Marshal(ordSale); err != nil {
				log.Panicln(err)
			} else {
				log.Println("PUBLISHING ORD SALE", string(out))
				lib.PublishEvent(context.Background(), "ordSale", string(out))
			}
		}
	}
}

func ParseScript(txo *lib.Txo) (listing *Listing) {
	script := *txo.Tx.Outputs[txo.Outpoint.Vout()].LockingScript
	sCryptPrefixIndex := bytes.Index(script, lib.SCryptPrefix)
	if sCryptPrefixIndex > -1 {
		ordLockSuffixIndex := bytes.Index(script, OrdLockSuffix)
		if ordLockSuffixIndex > -1 {
			ordLock := script[sCryptPrefixIndex+len(lib.SCryptPrefix) : ordLockSuffixIndex]
			if ordLockParts, err := bscript.DecodeParts(ordLock); err == nil && len(ordLockParts) > 0 {
				pkhash := lib.PKHash(ordLockParts[0])
				payOutput := &bt.Output{}
				_, err = payOutput.ReadFrom(bytes.NewReader(ordLockParts[1]))
				if err == nil {
					txo.PKHash = &pkhash
					listing = &Listing{
						PKHash: &pkhash,
						Price:  payOutput.Satoshis,
						PayOut: payOutput.Bytes(),
					}
				}
			}
		}
	}
	return
}

func (l *Listing) Save(t *lib.Txo) {
	if _, ok := t.Data["bsv20"]; !ok {
		_, err := lib.Db.Exec(context.Background(), `
			INSERT INTO listings(txid, vout, height, idx, price, payout, origin, oheight, oidx, spend, pkhash, data)
			SELECT $1, $2, t.height, t.idx, $3, $4, t.origin, o.height, o.idx, t.spend, t.pkhash, o.data
			FROM txos t
			JOIN txos o ON o.outpoint = t.origin
			WHERE t.txid=$1 AND t.vout=$2
			ON CONFLICT(txid, vout) DO UPDATE SET 
				height=EXCLUDED.height,
				idx=EXCLUDED.idx,
				origin=EXCLUDED.origin,
				oheight=EXCLUDED.oheight,
				oidx=EXCLUDED.oidx`,
			t.Outpoint.Txid(),
			t.Outpoint.Vout(),
			l.Price,
			l.PayOut,
		)

		if err != nil {
			log.Panicln(err)
		}
		event := &ListingEvent{
			Outpoint: t.Outpoint,
			PKHash:   l.PKHash,
			Price:    l.Price,
		}
		if out, err := json.Marshal(event); err == nil {
			log.Println("PUBLISHING ORD LISTING", string(out))
			lib.PublishEvent(context.Background(), "ordListing", string(out))
		}
	}
}
