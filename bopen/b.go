package bopen

import (
	"crypto/sha256"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/idx"
)

type BIndexer struct {
	idx.BaseIndexer
}

func (i *BIndexer) Tag() string {
	return "b"
}

func (i *BIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) (idxData *idx.IndexData) {
	txo := idxCtx.Txos[vout]
	if bopen, ok := txo.Data[ONESAT_LABEL]; ok {
		if b, ok := bopen.Data.(OneSat)[i.Tag()].(*File); ok {
			idxData = &idx.IndexData{
				Data: b,
			}
		}
	}
	return
}

func ParseB(scr *script.Script, idx *int) (b *File) {
	b = &File{}
	for i := 0; i < 4; i++ {
		prevIdx := *idx
		op, err := scr.ReadOp(idx)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*idx = prevIdx
			break
		}

		switch i {
		case 0:
			b.Content = op.Data
		case 1:
			b.Type = string(op.Data)
		case 2:
			b.Encoding = string(op.Data)
		case 3:
			b.Name = string(op.Data)
		}
	}
	hash := sha256.Sum256(b.Content)
	b.Size = uint32(len(b.Content))
	b.Hash = hash[:]
	return
}
