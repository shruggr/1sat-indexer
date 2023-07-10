package lib

import (
	"context"
	"log"
)

type Origin struct {
	Origin *Outpoint `json:"origin"`
	Num    uint64    `json:"num"`
	Txid   []byte    `json:"txid"`
	Vout   uint32    `json:"vout"`
	Height uint32    `json:"height"`
	Idx    uint64    `json:"idx"`
	Map    Map       `json:"MAP,omitempty"`
}

func (o *Origin) Save() {
	_, err := Db.Exec(context.Background(), `
		INSERT INTO origins(origin, txid, vout, height, idx, map)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT(origin) DO UPDATE SET
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			map=EXCLUDED.map`,
		o.Origin,
		o.Txid,
		o.Vout,
		o.Height,
		o.Idx,
		o.Map,
	)
	if err != nil {
		log.Panicf("Save Error: %s %+v\n", o.Origin, err)
	}
}
