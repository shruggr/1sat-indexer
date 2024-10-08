package bopen

import (
	"bytes"
	"regexp"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/lib"
)

const BOPEN = "bopen"

type BOpenIndexer struct {
	lib.BaseIndexer
}

func (i *BOpenIndexer) Tag() string {
	return BOPEN
}

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)

func (i *BOpenIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) (idxData *lib.IndexData) {
	txo := idxCtx.Txos[vout]
	scr := idxCtx.Tx.Outputs[vout].LockingScript

	start := 0
	if len(*scr) >= 25 && script.NewFromBytes((*scr)[:25]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[3:23])
		txo.Owners[pkhash.Address()] = struct{}{}
		start = 25
	}

	var opReturn int
	for i := start; i < len(*scr); {
		startI := i
		op, err := lib.ReadOp(*scr, &i)
		if err != nil {
			break
		}
		switch op.OpCode {
		case script.OpRETURN:
			if opReturn == 0 {
				opReturn = startI
			}
			bitcom, err := lib.ParseBitcom(idxCtx.Tx, vout, &i)
			if err != nil {
				continue
			}
			addInstance(txo, bitcom)

		case script.OpDATA1:
			if op.Data[0] == '|' && opReturn > 0 {
				bitcom, err := lib.ParseBitcom(idxCtx.Tx, vout, &i)
				if err != nil {
					continue
				}
				addInstance(txo, bitcom)
			}
		case script.OpDATA3:
			if i > 2 && bytes.Equal(op.Data, []byte("ord")) && (*scr)[startI-2] == 0 && (*scr)[startI-1] == script.OpIF {
				idxData = ParseInscription(txo, scr, &i)
			}
		}
	}
	return
}

func addInstance(txo *lib.Txo, instance interface{}) {
	if instance == nil {
		return
	}
	var bopen map[string]any
	if idxData, ok := txo.Data[BOPEN]; ok {
		bopen = idxData.Data.(map[string]any)
	} else {
		bopen = map[string]any{}
		txo.Data[BOPEN] = &lib.IndexData{
			Data: bopen,
		}
	}
	switch bo := instance.(type) {
	case *Sigma:
		var sigmas Sigmas
		if prev, ok := bopen["sigma"].(Sigmas); ok {
			sigmas = prev
		}
		sigmas = append(sigmas, bo)
	case Map:
		var m Map
		if prev, ok := bopen["map"].(Map); ok {
			m = prev
		}
		for k, v := range bo {
			m[k] = v
		}
	case *File:
		bopen["b"] = bo

	case *Inscription:
		bopen["insc"] = bo

	}
}

func (i *BOpenIndexer) PreSave(idxCtx *lib.IndexContext) {
	for _, txo := range idxCtx.Txos {
		delete(txo.Data, BOPEN)
	}
}
