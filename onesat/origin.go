package onesat

import (
	"encoding/json"

	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
)

const ORIGIN_TAG = "origin"

var TRIGGER = uint32(783968)

type Origin struct {
	Outpoint *lib.Outpoint `json:"outpoint"`
	Nonce    uint64        `json:"nonce"`
}

type OriginIndexer struct {
	idx.BaseIndexer
}

func (i *OriginIndexer) Tag() string {
	return ORIGIN_TAG
}

func (i *OriginIndexer) FromBytes(data []byte) (any, error) {
	obj := &Origin{}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *OriginIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]

	if *txo.Satoshis != 1 || idxCtx.Height < TRIGGER {
		return nil
	}

	deps := make([]*lib.Outpoint, 0)
	origin := &Origin{}
	satsIn := uint64(0)
	missing := false
	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == nil {
			missing = true
			break
		}
		deps = append(deps, spend.Outpoint)
		if satsIn == txo.OutAcc && *spend.Satoshis == 1 && spend.Height >= TRIGGER {
			if o, ok := spend.Data[ORIGIN_TAG]; ok {
				origin.Nonce = o.Data.(*Origin).Nonce + 1
				origin.Outpoint = o.Data.(*Origin).Outpoint
			}
			break
		} else if satsIn > txo.OutAcc {
			break
		}
		satsIn += *spend.Satoshis
	}

	if !missing && origin.Outpoint == nil {
		origin.Outpoint = txo.Outpoint
	}

	outpointEvent := &evt.Event{
		Id: "outpoint",
	}
	if origin.Outpoint != nil {
		outpointEvent.Value = origin.Outpoint.String()
	}
	return &idx.IndexData{
		Data:   origin,
		Deps:   deps,
		Events: []*evt.Event{outpointEvent},
	}
}
