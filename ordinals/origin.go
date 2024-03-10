package ordinals

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/shruggr/1sat-indexer/lib"
)

const MAX_DEPTH = 256

type Origin struct {
	Origin *lib.Outpoint `json:"origin"`
	Num    uint64        `json:"num"`
	Height *uint32       `json:"height"`
	Idx    uint64        `json:"idx"`
	Map    lib.Map       `json:"map,omitempty"`
}

func LoadOrigin(outpoint *lib.Outpoint, outAcc uint64) *lib.Outpoint {
	// fmt.Println("LoadOrigin", outpoint, outAcc)
	return calcOrigin(outpoint, outAcc, 0)
}

func calcOrigin(outpoint *lib.Outpoint, outAcc uint64, depth uint32) *lib.Outpoint {
	// fmt.Println("Finding", outpoint, outAcc, depth)
	if depth > MAX_DEPTH {
		return nil
		// log.Panicf("max depth exceeded %d %s\n", depth, outpoint)
	}
	origin := &lib.Outpoint{}
	row := Db.QueryRow(context.Background(),
		`SELECT origin FROM txos WHERE txid=$1 AND outacc=$2`,
		outpoint.Txid(),
		outAcc,
	)
	err := row.Scan(&origin)
	if err != nil && err != pgx.ErrNoRows {
		return nil
	} else if err == pgx.ErrNoRows || origin == nil {
		spends := lib.LoadSpends(outpoint.Txid(), nil)
		var inSats uint64
		for _, spend := range spends {
			if inSats < outAcc {
				inSats += spend.Satoshis
				continue
			}
			if inSats == outAcc && spend.Satoshis == 1 {
				origin = calcOrigin(spend.Outpoint, spend.OutAcc, depth+1)
				if origin != nil {
					spend.SetOrigin(origin)
					Rdb.Publish(context.Background(), origin.String(), outpoint.String())
				}
				return origin
			}
			break
		}
		return outpoint
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

func SaveMap(origin *lib.Outpoint) {
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

	m := lib.Map{}
	for rows.Next() {
		var data lib.Map
		err = rows.Scan(&data)
		if err != nil {
			log.Panicln(err)
		}
		for k, v := range data {
			m[k] = v
		}
	}

	_, err = Db.Exec(context.Background(), `
		INSERT INTO origins(origin, map)
		VALUES($1, $2)
		ON CONFLICT(origin) DO UPDATE SET
			map=EXCLUDED.map`,
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
		WHERE num = -1 AND height <= $1 AND height IS NOT NULL
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
		origin := &lib.Outpoint{}
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
