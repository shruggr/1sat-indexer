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

type Bsv21 struct {
	Id            *lib.Outpoint `json:"id,omitempty"`
	Op            string        `json:"op"`
	Symbol        *string       `json:"sym,omitempty"`
	Decimals      uint8         `json:"-"`
	Icon          *lib.Outpoint `json:"icon,omitempty"`
	Amt           uint64        `json:"amt"`
	Status        Bsv20Status   `json:"-"`
	Reason        *string       `json:"reason,omitempty"`
	PKHash        *lib.PKHash   `json:"owner,omitempty"`
	Price         uint64        `json:"price,omitempty"`
	PayOut        []byte        `json:"payout,omitempty"`
	Listing       bool          `json:"listing"`
	PricePerToken float64       `json:"pricePer,omitempty"`
	FundPath      string        `json:"-"`
	FundPKHash    []byte        `json:"-"`
	FundBalance   int           `json:"-"`
}

type Bsv21Indexer struct {
	lib.BaseIndexer
}

func (i *Bsv21Indexer) Tag() string {
	return "bsv21"
}

func (i *Bsv21Indexer) Parse(idxCtx *lib.IndexContext, vout uint32) (idxData *lib.IndexData) {
	txo := idxCtx.Txos[vout]
	bopen, ok := txo.Data[BOPEN]
	if !ok {
		return
	}
	bopenData, ok := bopen.Data.(map[string]any)
	if !ok {
		return
	}
	insc, ok := bopenData[i.Tag()].(*Inscription)
	if !ok || strings.ToLower(insc.File.Type) != "application/bsv-20" {
		return
	}
	if bsv21 := ParseBsv21Inscription(insc.File, txo); bsv21 == nil {
		return
	} else {
		return &lib.IndexData{
			Data: bsv21,
			Events: []*lib.Event{
				{
					Id:    "id",
					Value: bsv21.Id.String(),
				},
			},
		}
	}
}

func ParseBsv21Inscription(ord *File, txo *lib.Txo) (bsv21 *Bsv21) {
	data := map[string]string{}
	var err error
	if err := json.Unmarshal(ord.Content, &data); err != nil {
		return
	}
	var protocol string
	var ok bool
	if protocol, ok = data["p"]; !ok || protocol != "bsv-20" {
		return nil
	}
	if id, ok := data["id"]; ok {
		bsv21 = &Bsv21{}
		if bsv21.Id, err = lib.NewOutpointFromString(id); err != nil {
			return
		}
	} else {
		return
	}

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
			bsv21.Icon, _ = lib.NewOutpointFromString(icon)
		}
		bsv21.Id = txo.Outpoint
		hash := sha256.Sum256(*bsv21.Id)
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

	return bsv21
}
