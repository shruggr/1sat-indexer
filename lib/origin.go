package lib

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

const MAX_DEPTH = 256

type Origin struct {
	Origin *Outpoint `json:"origin"`
	Num    uint64    `json:"num"`
	Height *uint32   `json:"height"`
	Idx    uint64    `json:"idx"`
	Map    Map       `json:"map,omitempty"`
}

func LoadOrigin(outpoint *Outpoint, outAcc uint64) *Outpoint {
	return calcOrigin(outpoint, outAcc, 0)
}

func calcOrigin(outpoint *Outpoint, outAcc uint64, depth uint32) *Outpoint {
	// fmt.Println("Finding", outpoint, outAcc, depth)
	if depth > MAX_DEPTH {
		log.Panicf("max depth exceeded %d %s\n", depth, outpoint)
	}
	origin := &Outpoint{}
	row := Db.QueryRow(context.Background(),
		`SELECT origin FROM txos WHERE txid=$1 AND outacc=$2`,
		outpoint.Txid(),
		outAcc,
	)
	err := row.Scan(&origin)
	if err != nil && err != pgx.ErrNoRows {
		panic(err)
	} else if err == pgx.ErrNoRows || origin == nil {
		spends := LoadSpends(outpoint.Txid(), nil)
		var inSats uint64
		for _, spend := range spends {
			if inSats < outAcc {
				inSats += spend.Satoshis
				continue
			}
			if spend.Satoshis != 1 {
				origin = outpoint
			} else if inSats == outAcc {
				origin = calcOrigin(spend.Outpoint, spend.OutAcc, depth+1)
				spend.SetOrigin(origin)
			}
			break
		}
		// SetOrigin(outpoint, origin)
	}
	return origin
}

func (o *Origin) Save() {
	_, err := Db.Exec(context.Background(), `
		INSERT INTO origins(origin, height, idx, map)
		VALUES($1, $2, $3, $4)
		ON CONFLICT(origin) DO UPDATE SET
			height=EXCLUDED.height,
			idx=EXCLUDED.idx`,
		o.Origin,
		o.Height,
		o.Idx,
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
		WHERE origin=$1 AND data->>'map' IS NOT NULL
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
		log.Panicf("Save Error: %s %+v\n", origin, err)
	}
}

func SetOriginNum(height uint32) (err error) {

	row := Db.QueryRow(context.Background(),
		"SELECT MAX(num) FROM origins",
	)
	var dbNum sql.NullInt64
	err = row.Scan(&dbNum)
	if err != nil {
		log.Panic(err)
		return
	}
	var num uint64
	if dbNum.Valid {
		num = uint64(dbNum.Int64 + 1)
	}

	rows, err := Db.Query(context.Background(), `
		SELECT origin
		FROM origins
		WHERE num IS NULL AND height <= $1 AND height IS NOT NULL
		ORDER BY height, idx
		LIMIT 100`,
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
		// fmt.Printf("Origin Num %d %s\n", num, origin)
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
	// log.Println("Height", height, "Max Origin Num", num)
	return
}
