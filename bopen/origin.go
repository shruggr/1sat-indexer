package bopen

import (
	"encoding/json"

	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
)

const ORIGIN_TAG = "origin"

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
	var origin *Origin
	satsIn := uint64(0)
	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == nil {
			origin = &Origin{}
			break
		}
		deps = append(deps, spend.Outpoint)
		if satsIn < txo.OutAcc {
			satsIn += *spend.Satoshis
			continue
		}
		if satsIn == txo.OutAcc && *spend.Satoshis == 1 {
			if o, ok := spend.Data[ORIGIN_TAG]; ok {
				origin = o.Data.(*Origin)
			} else {
				origin = &Origin{}
				if spend.Height < TRIGGER {
					origin.Outpoint = txo.Outpoint
				}
			}
		}
		break
	}

	if origin == nil {
		origin = &Origin{
			Outpoint: txo.Outpoint,
			// Map:      Map{},
		}
	} else {
		origin.Nonce++
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
