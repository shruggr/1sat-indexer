package idx

import (
	"context"

	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type IndexDataMap map[string]*IndexData

type Txo struct {
	Outpoint *lib.Outpoint         `json:"outpoint"`
	Height   uint32                `json:"height"`
	Idx      uint64                `json:"idx"`
	Satoshis *uint64               `json:"satoshis,omitempty"`
	Script   []byte                `json:"script,omitempty"`
	OutAcc   uint64                `json:"-"`
	Owners   []string              `json:"owners,omitempty"`
	Events   []string              `json:"events,omitempty"`
	Data     map[string]*IndexData `json:"data,omitempty" msgpack:"-"`
	Score    float64               `json:"score,omitempty" msgpack:"-"`
	Spend    string                `json:"spend,omitempty" msgpack:"-"`
}

func (t *Txo) AddOwner(owner string) {
	for _, o := range t.Owners {
		if o == owner {
			return
		}
	}
	t.Owners = append(t.Owners, owner)
}

func (txo *Txo) LoadScript(ctx context.Context) error {
	if tx, err := jb.LoadTx(ctx, txo.Outpoint.TxidHex(), false); err != nil {
		return err
	} else {
		txo.Script = *tx.Outputs[txo.Outpoint.Vout()].LockingScript
	}
	return nil
}
