package ordinals

import (
	"context"
	"encoding/json"
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
