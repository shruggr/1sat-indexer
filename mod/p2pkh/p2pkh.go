package p2pkh

import (
	"encoding/json"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

const P2PKH_TAG = "p2pkh"

type P2PKH struct {
	Address string `json:"address"`
}

type P2PKHIndexer struct {
	idx.BaseIndexer
}

func (i *P2PKHIndexer) Tag() string {
	return P2PKH_TAG
}

func (i *P2PKHIndexer) FromBytes(data []byte) (any, error) {
	return P2PKHFromBytes(data)
}

func P2PKHFromBytes(data []byte) (*P2PKH, error) {
	obj := &P2PKH{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *P2PKHIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	output := idxCtx.Tx.Outputs[vout]

	if output.LockingScript.IsP2PKH() {
		if add, err := script.NewAddressFromPublicKeyHash((*output.LockingScript)[3:23], true); err == nil {
			// txo.AddOwner(add.AddressString)
			return &idx.IndexData{
				Data: &P2PKH{
					Address: add.AddressString,
				},
				Events: []*evt.Event{
					{
						Id:    "own",
						Value: add.AddressString,
					},
				},
			}
		}
	}
	return nil
}
