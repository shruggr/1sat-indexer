package onesat

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"strconv"
	"strings"

	bip32 "github.com/bsv-blockchain/go-sdk/compat/bip32"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

type Bsv20Status int

var idxHdKey, _ = bip32.GenerateHDKeyFromString("xpub661MyMwAqRbcF221R74MPqdipLsgUevAAX4hZP2rywyEeShpbe3v2r9ciAvSGT6FB22TEmFLdUyeEDJL4ekG8s9H5WXbzDQPr6eW1zEYYy9")

const (
	Invalid Bsv20Status = -1
	Pending Bsv20Status = 0
	Valid   Bsv20Status = 1
	MintFee uint64      = 100
)

const BSV20_INDEX_FEE = 1000

const BSV20_INCLUDE_FEE = 10000000

type Bsv20 struct {
	Ticker      string      `json:"tick,omitempty"`
	Op          string      `json:"op"`
	Max         uint64      `json:"-"`
	Limit       uint64      `json:"-"`
	Decimals    uint8       `json:"-"`
	Amt         *uint64     `json:"amt"`
	Implied     bool        `json:"-"`
	Status      Bsv20Status `json:"-"`
	Reason      *string     `json:"reason,omitempty"`
	FundPath    string      `json:"-"`
	FundPKHash  []byte      `json:"-"`
	FundBalance int         `json:"-"`
}

const BSV20_TAG = "bsv20"

type Bsv20Indexer struct {
	idx.BaseIndexer
	WhitelistFn *func(tick string) bool
	BlacklistFn *func(tick string) bool
}

func (i *Bsv20Indexer) Tag() string {
	return BSV20_TAG
}

func (i *Bsv20Indexer) Parse(idxCtx *idx.IndexContext, vout uint32) any {
	output := idxCtx.Outputs[vout]

	inscData, ok := output.Data[INSC_TAG]
	if !ok {
		return nil
	}
	insc, ok := inscData.(*Inscription)
	if !ok {
		return nil
	}
	if insc.JsonMap == nil || insc.File == nil || !(strings.HasPrefix(insc.File.Type, "application/bsv-20") || (strings.HasPrefix(insc.File.Type, "text/plain") && idxCtx.Height < 793000)) {
		return nil
	}
	protocol, ok := insc.JsonMap["p"]
	if !ok || protocol != "bsv-20" {
		return nil
	}
	tick, ok := insc.JsonMap["tick"]
	if !ok {
		return nil
	}

	if chars := []rune(tick); len(chars) > 4 {
		return nil
	}
	bsv20 := &Bsv20{
		Ticker: strings.ToUpper(tick),
	}

	if op, ok := insc.JsonMap["op"]; !ok {
		return nil
	} else {
		bsv20.Op = strings.ToLower(op)
	}

	if amt, ok := insc.JsonMap["amt"]; ok {
		if amt, err := strconv.ParseUint(amt, 10, 64); err != nil {
			return nil
		} else {
			bsv20.Amt = &amt
		}
	}

	if dec, ok := insc.JsonMap["dec"]; ok {
		if val, err := strconv.ParseUint(dec, 10, 8); err != nil || val > 18 {
			return nil
		} else {
			bsv20.Decimals = uint8(val)
		}
	}

	var err error
	switch bsv20.Op {
	case "deploy":
		if max, ok := insc.JsonMap["max"]; ok {
			if bsv20.Max, err = strconv.ParseUint(max, 10, 64); err != nil {
				return nil
			}
		}
		if limit, ok := insc.JsonMap["lim"]; ok {
			if bsv20.Limit, err = strconv.ParseUint(limit, 10, 64); err != nil {
				return nil
			}
		}
		hash := sha256.Sum256([]byte(bsv20.Ticker))
		path := fmt.Sprintf("21/%d/%d", binary.BigEndian.Uint32(hash[:8])>>1, binary.BigEndian.Uint32(hash[24:])>>1)
		ek, err := idxHdKey.DeriveChildFromPath(path)
		if err != nil {
			log.Panic(err)
		}
		pubKey, err := ek.ECPubKey()
		if err != nil {
			log.Panic(err)
		}
		bsv20.FundPath = path
		bsv20.FundPKHash = pubKey.Hash()
	case "mint", "transfer", "burn":
		if bsv20.Amt == nil {
			return nil
		}
	default:
		return nil
	}

	output.AddEvent(BSV20_TAG + ":tick:" + bsv20.Ticker)
	return bsv20
}
