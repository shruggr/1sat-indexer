package ordinals

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/shruggr/1sat-indexer/lib"
)

type Inscription struct {
	Outpoint  *lib.Outpoint   `json:"-"`
	Height    *uint32         `json:"-"`
	Idx       uint64          `json:"-"`
	Json      json.RawMessage `json:"json,omitempty"`
	Text      string          `json:"text,omitempty"`
	Words     []string        `json:"words,omitempty"`
	File      *lib.File       `json:"file,omitempty"`
	Pointer   *uint64         `json:"pointer,omitempty"`
	Parent    *lib.Outpoint   `json:"parent,omitempty"`
	Metadata  lib.Map         `json:"metadata,omitempty"`
	Metaproto []byte          `json:"metaproto,omitempty"`
}

func (i *Inscription) Save() {
	_, err := Db.Exec(context.Background(), `
		INSERT INTO inscriptions(outpoint, height, idx)
		VALUES($1, $2, $3)
		ON CONFLICT(outpoint) DO UPDATE SET
			height=EXCLUDED.height,
			idx=EXCLUDED.idx`,
		i.Outpoint,
		i.Height,
		i.Idx,
	)
	if err != nil {
		log.Panicf("Save Error: %s %+v\n", i.Outpoint, err)
	}
}

func SetInscriptionNum(height uint32) (err error) {
	row := Db.QueryRow(context.Background(),
		"SELECT MAX(num) FROM inscriptions",
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
		SELECT outpoint
		FROM inscriptions
		WHERE num = -1 AND height <= $1 AND height IS NOT NULL
		ORDER BY height, idx
		LIMIT 100000`,
		height,
	)
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		outpoint := &lib.Outpoint{}
		err = rows.Scan(&outpoint)
		if err != nil {
			log.Panic(err)
			return
		}
		// fmt.Printf("Inscription Num %d %d %s\n", num, height, outpoint)
		_, err = Db.Exec(context.Background(), `
			UPDATE inscriptions
			SET num=$2
			WHERE outpoint=$1`,
			outpoint, num,
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
