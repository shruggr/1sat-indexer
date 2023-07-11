package lib

import "context"

type Listing struct {
	Txid   []byte  `json:"-"`
	Vout   uint32  `json:"-"`
	Height *uint32 `json:"-"`
	Idx    uint64  `json:"-"`
	Price  uint64  `json:"price"`
	PayOut []byte  `json:"payout"`
}

func (l *Listing) Save() (err error) {
	_, err = Db.Exec(context.Background(), `
		INSERT INTO listings(txid, vout, height, idx, price, payout, origin, num, spend, pkhash, map, bsv20)
		SELECT $1, $2, $3, $4, $5, $6, t.origin, o.num, t.spend, t.pkhash, o.map, t.bsv20
		FROM txos t
		JOIN origin i ON i.origin = t.origin
		WHERE t.txid=$1 AND t.vout=$2
		ON CONFLICT(txid, vout) DO UPDATE SET 
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			origin=EXCLUDED.origin`,
		l.Txid,
		l.Vout,
		l.Height,
		l.Idx,
		l.Price,
		l.PayOut,
	)

	if err != nil {
		return
	}
	return
}
