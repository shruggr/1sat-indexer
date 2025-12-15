package bitcom

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var MAP_PROTO = "1PuQa7K62MiKCtssSLKy1kh56WWU7MtUR5"

const MAP_TAG = "map"

type Map map[string]interface{}

func (m Map) Merge(m2 Map) Map {
	r := make(Map, len(m)+len(m2))
	for k, v := range m {
		r[k] = v
	}
	for k, v := range m2 {
		r[k] = v
	}
	return r
}

type MapIndexer struct {
	idx.BaseIndexer
}

func (i *MapIndexer) Tag() string {
	return MAP_TAG
}

func (i *MapIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) any {
	output := idxCtx.Outputs[vout]
	if bitcomData, ok := output.Data[BITCOM_TAG]; ok {
		mp := Map{}
		for _, b := range bitcomData.([]*Bitcom) {
			if b.Protocol == MAP_PROTO {
				ParseMAP(mp, script.NewFromBytes(b.Script), 0)
			}
		}
		if len(mp) > 0 {
			return mp
		}
	}
	return nil
}

func ParseMAP(mp Map, scr *script.Script, idx int) {
	pos := &idx
	op, err := scr.ReadOp(pos)
	if err != nil {
		return
	}
	if string(op.Data) != "SET" {
		return
	}
	for {
		prevIdx := *pos
		op, err = scr.ReadOp(pos)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*pos = prevIdx
			break
		}
		opKey := strings.Replace(string(bytes.Replace(op.Data, []byte{0}, []byte{' '}, -1)), "\\u0000", " ", -1)
		prevIdx = *pos
		op, err = scr.ReadOp(pos)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*pos = prevIdx
			break
		}

		if (len(opKey) == 1 && opKey[0] == 0) || len(opKey) > 256 || len(op.Data) > 1024 {
			continue
		}

		if !utf8.Valid([]byte(opKey)) || !utf8.Valid(op.Data) {
			continue
		}

		mp[opKey] = strings.Replace(string(bytes.Replace(op.Data, []byte{0}, []byte{' '}, -1)), "\\u0000", " ", -1)

	}
	if val, ok := mp["subTypeData"].(string); ok {
		if bytes.Contains([]byte(val), []byte{0}) || bytes.Contains([]byte(val), []byte("\\u0000")) {
			delete(mp, "subTypeData")
		} else {
			var subTypeData json.RawMessage
			if err := json.Unmarshal([]byte(val), &subTypeData); err == nil {
				mp["subTypeData"] = subTypeData
			}
		}
	}
}
