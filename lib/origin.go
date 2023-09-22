package lib

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

type Origin struct {
	Origin *Outpoint `json:"origin"`
	Num    uint64    `json:"num"`
	Txid   []byte    `json:"txid"`
	Vout   uint32    `json:"vout"`
	Height uint32    `json:"height"`
	Idx    uint64    `json:"idx"`
	Data   *TxoData  `json:"data,omitempty"`
	Map    Map       `json:"map,omitempty"`
}

func (o *Origin) Save() {
	_, err := Db.Exec(context.Background(), `
		INSERT INTO origins(origin, txid, vout, height, idx, data, map)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT(origin) DO UPDATE SET
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			data=EXCLUDED.data`,
		o.Origin,
		o.Txid,
		o.Vout,
		o.Height,
		o.Idx,
		o.Data,
		o.Map,
	)
	if err != nil {
		log.Panicf("Save Error: %s %+v\n", o.Origin, err)
	}
}

func SaveMap(origin *Outpoint) {
	rows, err := Db.Query(context.Background(), `
		SELECT data->>'map'
		FROM txos
		WHERE origin=$1
		ORDER BY height ASC, idx ASC, vout ASC`,
		origin,
	)
	if err != nil {
		log.Panicf("BuildMap Error: %s %+v\n", origin, err)
	}
	rows.Close()

	m := map[string]interface{}{}
	for rows.Next() {
		var data Map
		err = rows.Scan(&data)
		if err != nil {
			panic(err)
		}
		for k, v := range data {
			m[k] = v
		}
	}

	_, err = Db.Exec(context.Background(), `
		UPDATE origins
		SET map=$2
		WHERE origin=$1`,
		origin,
		m,
	)
	if err != nil {
		log.Panicf("Save Error: %s %+v\n", o.Origin, err)
	}
	return
}

func SetOriginNum(height uint32) (err error) {
	rows, err := Db.Query(context.Background(),
		"SELECT MAX(num) FROM origins",
	)
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	var num uint64
	if rows.Next() {
		var dbNum sql.NullInt64
		err = rows.Scan(&dbNum)
		if err != nil {
			log.Panic(err)
			return
		}
		if dbNum.Valid {
			num = uint64(dbNum.Int64 + 1)
		}
	} else {
		return
	}

	rows, err = Db.Query(context.Background(), `
		SELECT origin
		FROM origins
		WHERE num IS NULL AND height <= $1 AND height IS NOT NULL
		ORDER BY height, idx, vout`,
		height,
	)
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		origin := &Outpoint{}
		err = rows.Scan(&origin)
		if err != nil {
			log.Panic(err)
			return
		}
		fmt.Printf("Origin Num %d %s\n", num, origin)
		_, err = Db.Exec(context.Background(), `
			UPDATE origins
			SET num=$2
			WHERE origin=$1`,
			origin, num,
		)
		if err != nil {
			log.Panic(err)
			return
		}
		num++
	}
	Rdb.Publish(context.Background(), "inscriptionNum", fmt.Sprintf("%d", num))
	return
}
