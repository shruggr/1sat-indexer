package bopen

import (
	"encoding/json"

	"github.com/shruggr/1sat-indexer/lib"
)

const MAX_DEPTH = 256
const ORIGIN_TAG = "origin"

type Origin struct {
	Outpoint *lib.Outpoint `json:"outpoint"`
	Nonce    uint64        `json:"nonce"`
	MimeType string        `json:"type,omitempty"`
	// Inscription *Inscription `json:"insc,omitempty"`
	// Map Map `json:"map,omitempty"`
}

type OriginIndexer struct {
	lib.BaseIndexer
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

func (i *OriginIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) *lib.IndexData {
	txo := idxCtx.Txos[vout]

	if txo.Satoshis != 1 || idxCtx.Height < lib.TRIGGER {
		return nil
	}

	if len(idxCtx.Spends) == 0 {
		idxCtx.ParseSpends()
	}
	inSats := uint64(0)
	idxData := &lib.IndexData{
		Deps: make([]*lib.Outpoint, 0),
	}
	var origin *Origin
	for _, spend := range idxCtx.Spends {
		// if spend.Satoshis == nil {
		// 	idxCtx.QueueDependency(spend.Outpoint)
		// 	return
		// } else if len(idxData.DepQueue) > 0 {
		// 	continue
		// } else {
		idxData.Deps = append(idxData.Deps, spend.Outpoint)
		// }
		if inSats < txo.OutAcc {
			inSats += spend.Satoshis
			continue
		}
		if inSats == txo.OutAcc && spend.Satoshis == 1 {
			if o, ok := spend.Data[ORIGIN_TAG]; ok {
				origin = o.Data.(*Origin)
				// } else {
				// 	idxData.DepQueue = append(idxData.DepQueue, spend.Outpoint)
				// 	return idxData
			}
		}
		break
	}
	// if len(idxData.DepQueue) > 0 {
	// 	return idxData
	// }
	if origin == nil {
		origin = &Origin{
			Outpoint: txo.Outpoint,
			// Map:      Map{},
		}
	} else {
		origin.Nonce++
	}
	if idxData, ok := txo.Data[INSCRIPTION_TAG]; ok {
		insc := idxData.Data.(*Inscription)
		if insc.File != nil && insc.File.Type != "" {
			origin.MimeType = insc.File.Type
		}
	}
	// if idxData, ok := txo.Data[MAP_TAG]; ok {
	// 	mp := idxData.Data.(Map)
	// 	if origin.Map == nil {
	// 		origin.Map = mp
	// 	} else {
	// 		for k, v := range mp {
	// 			origin.Map[k] = v
	// 		}
	// 	}
	// }

	return &lib.IndexData{
		Data: origin,
		Events: []*lib.Event{
			{
				Id:    "outpoint",
				Value: origin.Outpoint.String(),
			},
		},
	}

	// if origin, err := i.calculateOrigin(idxCtx, uint32(vout), 0); err != nil {
	// 	log.Panicln(err)
	// 	return nil
	// } else {
	// 	idxData := &lib.IndexData{
	// 		Data: origin,
	// 	}
	// 	if origin != nil {
	// 		idxData.Events = append(idxData.Events, &lib.Event{
	// 			Id:    "outpoint",
	// 			Value: origin.Outpoint.String(),
	// 		})
	// 	}
	// 	return idxData
	// }
}

// func (i *OriginIndexer) calculateOrigin(idxCtx *lib.IndexContext, vout uint32, depth uint) (origin *Origin, err error) {
// 	if depth > MAX_DEPTH {
// 		log.Panic("Max Depth Exceeded")
// 		return nil, nil
// 	}
// 	txo := idxCtx.Txos[vout]
// 	inSats := uint64(0)
// 	for _, spend := range idxCtx.Spends {
// 		if inSats < txo.OutAcc {
// 			inSats += spend.Satoshis
// 			continue
// 		}
// 		if inSats == txo.OutAcc && spend.Satoshis == 1 {
// 			if o, ok := spend.Data[i.Tag()]; ok {
// 				return o.Data.(*Origin), nil
// 			}
// 			if tx, err := lib.LoadTx(idxCtx.Ctx, spend.Outpoint.TxidHex()); err != nil {
// 				log.Panicln(err)
// 				return nil, err
// 			} else {
// 				spendCtx := lib.NewIndexContext(idxCtx.Ctx, tx, nil, false)
// 				spendCtx.ParseTxn()
// 				if parent, err := i.calculateOrigin(spendCtx, spend.Outpoint.Vout(), depth+1); err != nil {
// 					return nil, err
// 				} else {
// 					origin := *parent
// 					origin.Nonce++
// 					spend.Data[ORIGIN_TAG] = &lib.IndexData{
// 						Data: &origin,
// 					}
// 					spend.SaveData(idxCtx.Ctx, []string{ORIGIN_TAG})
// 					return &origin, nil
// 				}
// 			}
// 		}
// 		break

// 	}

// 	origin = &Origin{
// 		Outpoint: txo.Outpoint,
// 	}
// 	return origin, nil
// }
