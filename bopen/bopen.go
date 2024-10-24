package bopen

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
)

const ONESAT_LABEL = "1sat"

type OneSat map[string]any

func (b *OneSat) addInstance(instance interface{}) {
	switch bo := instance.(type) {
	case *Sigma:
		var sigmas Sigmas
		if prev, ok := (*b)["sigma"].(Sigmas); ok {
			sigmas = prev
		}
		sigmas = append(sigmas, bo)
		(*b)["sigma"] = sigmas
	case Map:
		var m Map
		if prev, ok := (*b)["map"].(Map); ok {
			m = prev
			for k, v := range bo {
				m[k] = v
			}
		} else {
			m = bo
		}
		(*b)["map"] = m
	case *File:
		(*b)["b"] = bo

	case *Inscription:
		(*b)["insc"] = bo
		if strings.ToLower(bo.File.Type) == "application/bsv-20" && bo.Json != nil {
			data := map[string]string{}
			json.Unmarshal(bo.File.Content, &data)
			(*b)[BSV20_TAG] = data
		}
	}
}

type BOpenIndexer struct {
	idx.BaseIndexer
}

func (i *BOpenIndexer) Tag() string {
	return ONESAT_LABEL
}

func (i *BOpenIndexer) FromBytes(data []byte) (any, error) {
	obj := OneSat{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)

func (i *BOpenIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]
	scr := idxCtx.Tx.Outputs[vout].LockingScript

	bopen := make(OneSat)

	start := 0
	if len(*scr) >= 25 && script.NewFromBytes((*scr)[:25]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[3:23])
		txo.AddOwner(pkhash.Address())
		start = 25
	}

	var opReturn int
	for i := start; i < len(*scr); {
		startI := i
		op, err := scr.ReadOp(&i)
		if err != nil {
			break
		}
		switch op.Op {
		case script.OpRETURN:
			if opReturn == 0 {
				opReturn = startI
			}
			bitcom, err := ParseBitcom(idxCtx.Tx, vout, &i)
			if err != nil {
				continue
			}
			bopen.addInstance(bitcom)

		case script.OpDATA1:
			if op.Data[0] == '|' && opReturn > 0 {
				bitcom, err := ParseBitcom(idxCtx.Tx, vout, &i)
				if err != nil {
					continue
				}
				bopen.addInstance(bitcom)
			}
		case script.OpDATA3:
			if i > 2 && bytes.Equal(op.Data, []byte("ord")) && (*scr)[startI-2] == 0 && (*scr)[startI-1] == script.OpIF {
				insc := ParseInscription(txo, scr, &i, bopen)
				bopen.addInstance(insc)
			}
		}
	}
	if len(bopen) > 0 {
		return &idx.IndexData{
			Data: bopen,
		}
	}
	return nil
}

func (i *BOpenIndexer) PreSave(idxCtx *idx.IndexContext) {
	for _, txo := range idxCtx.Txos {
		if txo.Data[ONESAT_LABEL] != nil {
			delete(txo.Data, ONESAT_LABEL)
		}
	}
}
