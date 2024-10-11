package bopen

import (
	"bytes"
	"regexp"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/lib"
)

const BOPEN = "bopen"

type BOpen map[string]any

func (b *BOpen) addInstance(instance interface{}) {
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
	}
}

type BOpenIndexer struct {
	lib.BaseIndexer
}

func (i *BOpenIndexer) Tag() string {
	return BOPEN
}

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)

func (i *BOpenIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) *lib.IndexData {
	txo := idxCtx.Txos[vout]
	scr := idxCtx.Tx.Outputs[vout].LockingScript

	bopen := make(BOpen)

	start := 0
	if len(*scr) >= 25 && script.NewFromBytes((*scr)[:25]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[3:23])
		txo.AddOwner(pkhash.Address())
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
		return &lib.IndexData{
			Data: bopen,
		}
	}
	return nil
}

func (i *BOpenIndexer) PreSave(idxCtx *lib.IndexContext) {
	for _, txo := range idxCtx.Txos {
		if txo.Data[BOPEN] != nil {
			delete(txo.Data, BOPEN)
		}
	}
}
