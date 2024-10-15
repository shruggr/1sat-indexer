package bopen

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/libsv/go-bk/crypto"
	"github.com/shruggr/1sat-indexer/lib"
)

const BSV21_INDEX_FEE = 1000
const BSV21_TAG = "bsv21"

type Bsv21 struct {
	Id            string      `json:"id,omitempty"`
	Op            string      `json:"op"`
	Symbol        *string     `json:"sym,omitempty"`
	Decimals      uint8       `json:"dec"`
	Icon          string      `json:"icon,omitempty"`
	Amt           uint64      `json:"amt"`
	Status        Bsv20Status `json:"status"`
	Reason        *string     `json:"reason,omitempty"`
	Price         uint64      `json:"price,omitempty"`
	PayOut        []byte      `json:"payout,omitempty"`
	Listing       bool        `json:"listing"`
	PricePerToken float64     `json:"pricePer,omitempty"`
	FundPath      string      `json:"-"`
	FundPKHash    []byte      `json:"fundAddress,omitempty"`
	FundBalance   int         `json:"-"`
}

type Bsv21Indexer struct {
	lib.BaseIndexer
	WhitelistFn *func(tokenId string) bool
	BlacklistFn *func(tokenId string) bool
}

func (i *Bsv21Indexer) Tag() string {
	return BSV21_TAG
}

func (i *Bsv21Indexer) FromBytes(data []byte) (any, error) {
	obj := &Bsv21{}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *Bsv21Indexer) Parse(idxCtx *lib.IndexContext, vout uint32) *lib.IndexData {
	txo := idxCtx.Txos[vout]

	var err error
	if idxData, ok := txo.Data[BOPEN_TAG]; !ok {
		return nil
	} else if bopen, ok := idxData.Data.(BOpen); !ok {
		return nil
	} else if data, ok := bopen[BSV20_TAG].(map[string]string); !ok {
		return nil
	} else if protocol, ok := data["p"]; !ok || protocol != "bsv-20" {
		return nil
	} else if id, ok := data["id"]; !ok {
		return nil
	} else if _, err = lib.NewOutpointFromString(id); err != nil {
		return nil
	} else {
		bsv21 := &Bsv21{
			Id: id,
		}
		// if i.WhitelistFn != nil && !(*i.WhitelistFn)(bsv21.Id) {
		// 	return nil
		// } else if i.BlacklistFn != nil || (*i.BlacklistFn)(bsv21.Id) {
		// 	return nil
		// } else
		if op, ok := data["op"]; ok {
			bsv21.Op = strings.ToLower(op)
		} else {
			return nil
		}

		if amt, ok := data["amt"]; ok {
			if bsv21.Amt, err = strconv.ParseUint(amt, 10, 64); err != nil {
				return nil
			}
		}

		if dec, ok := data["dec"]; ok {
			var val uint64
			if val, err = strconv.ParseUint(dec, 10, 8); err != nil || val > 18 {
				return nil
			}
			bsv21.Decimals = uint8(val)
		}

		switch bsv21.Op {
		case "deploy+mint":
			if sym, ok := data["sym"]; ok {
				bsv21.Symbol = &sym
			}
			bsv21.Status = Valid
			if icon, ok := data["icon"]; ok {
				if strings.HasPrefix(icon, "_") {
					icon = fmt.Sprintf("%x%s", txo.Outpoint.Txid(), icon)
				}
				bsv21.Icon = icon
			}
			bsv21.Id = txo.Outpoint.String()
			hash := sha256.Sum256(*txo.Outpoint)
			path := fmt.Sprintf("21/%d/%d", binary.BigEndian.Uint32(hash[:8])>>1, binary.BigEndian.Uint32(hash[24:])>>1)
			ek, err := idxHdKey.DeriveChildFromPath(path)
			if err != nil {
				log.Panic(err)
			}
			pubKey, err := ek.ECPubKey()
			if err != nil {
				log.Panic(err)
			}
			bsv21.FundPath = path
			bsv21.FundPKHash = crypto.Hash160(pubKey.SerialiseCompressed())
		case "transfer", "burn":
		default:
			return nil
		}

		return &lib.IndexData{
			Data: bsv21,
			Events: []*lib.Event{
				{
					Id:    "id",
					Value: bsv21.Id,
				},
			},
		}
	}
}

type bsv21Token struct {
	balance uint64
	reason  *string
	token   *Bsv21
	outputs []*lib.IndexData
	deps    []*lib.Outpoint
}
type bsv21Ctx struct {
	tokens map[string]*bsv21Token
}

func (i *Bsv21Indexer) PreSave(idxCtx *lib.IndexContext) {
	ctx := bsv21Ctx{
		tokens: map[string]*bsv21Token{},
	}

	for _, txo := range idxCtx.Txos {
		if idxData, ok := txo.Data[BSV21_TAG]; ok {
			if bsv21, ok := idxData.Data.(*Bsv21); ok {
				if token, ok := ctx.tokens[bsv21.Id]; !ok {
					token = &bsv21Token{
						outputs: []*lib.IndexData{
							idxData,
						},
					}
					ctx.tokens[bsv21.Id] = token
				} else {
					token.outputs = append(token.outputs, idxData)
				}
			}
		}
	}
	if len(ctx.tokens) == 0 {
		return
	}

	// if len(idxCtx.Spends) == 0 {
	// 	idxCtx.ParseSpends()
	// }

	hasDepQueue := false
	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == nil {
			hasDepQueue = true
			idxCtx.QueueDependency(spend.Outpoint.TxidHex())
			continue
		} else if hasDepQueue {
			continue
		}
		if idxData, ok := spend.Data[BSV21_TAG]; ok {
			if bsv21, ok := idxData.Data.(*Bsv21); ok {
				switch bsv21.Status {
				case Valid:
					if token, ok := ctx.tokens[bsv21.Id]; ok {
						token.balance += bsv21.Amt
						token.token = bsv21
						token.deps = append(token.deps, spend.Outpoint)
					}
				case Pending:
					log.Panicln("Pending BSV21", bsv21.Id)
				}
			}
		}
	}
	if hasDepQueue {
		return
	}
	reasons := map[string]string{}
	for _, token := range ctx.tokens {
		for _, idxData := range token.outputs {
			if bsv21, ok := idxData.Data.(*Bsv21); ok {
				if token.token == nil {
					reason := "missing inputs"
					token.reason = &reason
				} else if bsv21.Amt > token.balance {
					reason := "insufficient funds"
					token.reason = &reason
				} else {
					bsv21.Icon = token.token.Icon
					bsv21.Symbol = token.token.Symbol
					bsv21.Decimals = token.token.Decimals
					token.balance -= bsv21.Amt
				}
			}
		}
	}
	for tokenId, token := range ctx.tokens {
		for _, idxData := range token.outputs {
			if bsv21, ok := idxData.Data.(*Bsv21); ok {
				if reason, ok := reasons[tokenId]; ok {
					bsv21.Status = Invalid
					bsv21.Reason = &reason
					idxData.Events = append(idxData.Events, &lib.Event{
						Id:    "invalid",
						Value: tokenId,
					})
					idxData.Deps = append(idxData.Deps, token.deps...)
				} else {
					bsv21.Status = Valid
					idxData.Events = append(idxData.Events, &lib.Event{
						Id:    "valid",
						Value: tokenId,
					})
					idxData.Deps = append(idxData.Deps, token.deps...)
				}
			}
		}
	}
}
