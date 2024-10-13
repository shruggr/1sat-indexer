package bopen

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/lib"
)

const MAP_TAG = "map"

type Map map[string]interface{}

type MapIndexer struct {
	lib.BaseIndexer
}

func (i *MapIndexer) Tag() string {
	return MAP_TAG
}

func (i *MapIndexer) FromBytes(data []byte) (any, error) {
	obj := Map{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *MapIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) (idxData *lib.IndexData) {
	txo := idxCtx.Txos[vout]
	if bopen, ok := txo.Data[BOPEN_TAG]; ok {
		if m, ok := bopen.Data.(BOpen)[MAP_TAG].(Map); ok {
			idxData = &lib.IndexData{
				Data: m,
			}
		}
	}
	return
}

func ParseMAP(scr *script.Script, idx *int) (mp Map) {
	op, err := scr.ReadOp(idx)
	if err != nil {
		return
	}
	if string(op.Data) != "SET" {
		return nil
	}
	mp = Map{}
	for {
		prevIdx := *idx
		op, err = scr.ReadOp(idx)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*idx = prevIdx
			break
		}
		opKey := strings.Replace(string(bytes.Replace(op.Data, []byte{0}, []byte{' '}, -1)), "\\u0000", " ", -1)
		prevIdx = *idx
		op, err = scr.ReadOp(idx)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*idx = prevIdx
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

	return
}
