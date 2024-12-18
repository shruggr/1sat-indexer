package onesat

import (
	"encoding/json"

	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
	"github.com/shruggr/1sat-indexer/v5/mod/bitcom"
)

const ORIGIN_TAG = "origin"

var TRIGGER = uint32(783968)

type Origin struct {
	Outpoint *lib.Outpoint `json:"outpoint"`
	Nonce    uint64        `json:"nonce"`
	Parent   *lib.Outpoint `json:"parent,omitempty"`
	Type     string        `json:"type,omitempty"`
	Map      bitcom.Map    `json:"map,omitempty"`
}

type OriginIndexer struct {
	idx.BaseIndexer
}

func (i *OriginIndexer) Tag() string {
	return ORIGIN_TAG
}

func (i *OriginIndexer) FromBytes(data []byte) (any, error) {
	return NewOriginFromBytes(data)
}

func NewOriginFromBytes(data []byte) (*Origin, error) {
	obj := &Origin{}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *OriginIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]

	if *txo.Satoshis != 1 || (idxCtx.Height < TRIGGER && idxCtx.Height != 0) {
		return nil
	}

	origin := &Origin{}
	inscData := txo.Data[INSC_TAG]
	if inscData != nil {
		insc := inscData.Data.(*Inscription)
		if insc.File != nil {
			origin.Type = insc.File.Type
		}
	}
	mapData := txo.Data[bitcom.MAP_TAG]
	if mapData != nil {
		origin.Map = mapData.Data.(bitcom.Map)
	}
	events := make([]*evt.Event, 0)
	deps := make([]*lib.Outpoint, 0)
	satsIn := uint64(0)
	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == nil {
			break
		}
		deps = append(deps, spend.Outpoint)
		if satsIn == txo.OutAcc && *spend.Satoshis == 1 && spend.Height >= TRIGGER {
			origin.Parent = spend.Outpoint
			events = append(events, &evt.Event{
				Id:    "parent",
				Value: spend.Outpoint.String(),
			})
			if o, ok := spend.Data[ORIGIN_TAG]; ok {
				parent := o.Data.(*Origin)
				origin.Nonce = parent.Nonce + 1
				origin.Outpoint = parent.Outpoint
				// origin.Inscription = parent.Inscription
				if origin.Map == nil {
					origin.Map = parent.Map
				} else if parent.Map != nil {
					origin.Map = parent.Map.Merge(origin.Map)
				}
			}
			break
		}
		satsIn += *spend.Satoshis
		if satsIn > txo.OutAcc {
			origin.Outpoint = txo.Outpoint
			break
		}
	}

	var outpoint string
	if origin.Outpoint != nil {
		outpoint = origin.Outpoint.String()
	}
	events = append(events, &evt.Event{
		Id:    "outpoint",
		Value: outpoint,
	})
	return &idx.IndexData{
		Data:   origin,
		Deps:   deps,
		Events: events,
	}
}
