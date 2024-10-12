package bopen

import (
	"encoding/json"
	"log"

	"github.com/shruggr/1sat-indexer/lib"
)

const MAX_DEPTH = 256
const ORIGIN_TAG = "origin"

type Origin struct {
	Outpoint    *lib.Outpoint `json:"outpoint"`
	Nonce       uint64        `json:"nonce"`
	Map         Map           `json:"map"`
	Inscription *Inscription  `json:"insc,omitempty"`
	Sigmas      *Sigmas       `json:"sigma,omitempty"`
}

type OriginIndexer struct {
	lib.BaseIndexer
}

func (i *OriginIndexer) Tag() string {
	return ORIGIN_TAG
}

func (i *OriginIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) *lib.IndexData {
	txo := idxCtx.Txos[vout]

	if txo.Satoshis != 1 {
		return nil
	}

	if origin, err := i.calculateOrigin(idxCtx, uint32(vout), 0); err != nil {
		log.Panicln(err)
		return nil
	} else {
		idxData := &lib.IndexData{
			Data: origin,
		}
		if origin != nil {
			idxData.Events = append(idxData.Events, &lib.Event{
				Id:    "outpoint",
				Value: origin.Outpoint.String(),
			})
		}
		return idxData
	}
}

// func (oi *OriginIndexer) PreSave(idxCtx *lib.IndexContext) error {
// 	tag := oi.Tag()
// 	for vout, txo := range idxCtx.Txos {
// 		if txo.Data[tag] == nil {
// 			continue
// 		}
// 		if txo.Data[tag].PostProcess {
// 			if origin, err := oi.calculateOrigin(idxCtx, uint32(vout), 0); err != nil {
// 				return err
// 			} else {
// 				txo.Data[tag].Data = origin
// 			}
// 		}
// 	}
// 	return nil
// }

func (i *OriginIndexer) calculateOrigin(idxCtx *lib.IndexContext, vout uint32, depth uint) (origin *Origin, err error) {
	if depth > MAX_DEPTH {
		log.Panic("Max Depth Exceeded")
		return nil, nil
	}
	txo := idxCtx.Txos[vout]
	inSats := uint64(0)
	for _, spend := range idxCtx.Spends {
		if inSats < txo.OutAcc {
			inSats += spend.Satoshis
			continue
		}
		if inSats == txo.OutAcc && spend.Satoshis == 1 {
			if o, ok := spend.Data[i.Tag()]; ok {
				switch oData := o.Data.(type) {
				case *Origin:
					return oData, nil
				case json.RawMessage:
					origin := &Origin{}
					if err := json.Unmarshal(oData, origin); err != nil {
						return nil, err
					}
					origin.Nonce++
					return origin, nil
				default:
					log.Panic("Unknown Origin Data Type")
				}
			}
			if tx, err := lib.LoadTx(idxCtx.Ctx, spend.Outpoint.TxidHex()); err != nil {
				log.Panicln(err)
				return nil, err
			} else {
				spendCtx := lib.NewIndexContext(idxCtx.Ctx, tx, nil)
				spendCtx.ParseTxn()
				if parent, err := i.calculateOrigin(spendCtx, spend.Outpoint.Vout(), depth+1); err != nil {
					return nil, err
				} else {
					origin := *parent
					origin.Nonce++
					spend.Data[ORIGIN_TAG] = &lib.IndexData{
						Data: &origin,
					}
					spend.SaveData(idxCtx.Ctx, []string{ORIGIN_TAG})
					return &origin, nil
				}
			}
		}
		break

	}

	origin = &Origin{
		Outpoint: txo.Outpoint,
		Map:      make(Map),
	}
	return origin, nil
}

// func LoadOrigin(outpoint *lib.Outpoint, outAcc uint64) *lib.Outpoint {
// 	// fmt.Println("LoadOrigin", outpoint, outAcc)
// 	return calcOrigin(outpoint, outAcc, 0)
// }

// func calcOrigin(outpoint *lib.Outpoint, outAcc uint64, depth uint32) *lib.Outpoint {
// 	// fmt.Println("Finding", outpoint, outAcc, depth)
// 	if depth > MAX_DEPTH {
// 		return nil
// 		// log.Panicf("max depth exceeded %d %s\n", depth, outpoint)
// 	}
// 	origin := &lib.Outpoint{}
// 	row := lib.Db.QueryRow(context.Background(),
// 		`SELECT origin FROM txos WHERE txid=$1 AND outacc=$2`,
// 		outpoint.Txid(),
// 		outAcc,
// 	)
// 	err := row.Scan(&origin)
// 	if err != nil && err != pgx.ErrNoRows {
// 		return nil
// 	} else if err == pgx.ErrNoRows || origin == nil {
// 		spends := lib.LoadSpends(outpoint.Txid(), nil)
// 		var inSats uint64
// 		for _, spend := range spends {
// 			if inSats < outAcc {
// 				inSats += spend.Satoshis
// 				continue
// 			}
// 			if inSats == outAcc && spend.Satoshis == 1 {
// 				origin = calcOrigin(spend.Outpoint, spend.OutAcc, depth+1)
// 				if origin != nil {
// 					spend.SetOrigin(origin)
// 					lib.PublishEvent(context.Background(), origin.String(), outpoint.String())
// 				}
// 				return origin
// 			}
// 			break
// 		}
// 		return outpoint
// 	}
// 	return origin
// }
