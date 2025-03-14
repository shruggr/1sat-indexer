package bitcom

import (
	"crypto/sha256"

	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

var B_PROTO = "19HxigV4QyBv3tHpQVcUEQyq1pzZVdoAut"

type BIndexer struct {
	idx.BaseIndexer
}

func (i *BIndexer) Tag() string {
	return "b"
}

func (i *BIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) (idxData *idx.IndexData) {
	txo := idxCtx.Txos[vout]
	if bitcomData, ok := txo.Data[BITCOM_TAG]; ok {
		for _, b := range bitcomData.Data.([]*Bitcom) {
			if b.Protocol == B_PROTO {
				b := ParseB(script.NewFromBytes(b.Script), 0)
				if b != nil {
					idxData = &idx.IndexData{
						Data: b,
					}
					break
				}
			}
		}
	}
	return
}

func ParseB(scr *script.Script, idx int) (b *lib.File) {
	pos := &idx
	b = &lib.File{}
	for i := 0; i < 4; i++ {
		prevIdx := *pos
		op, err := scr.ReadOp(pos)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*pos = prevIdx
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
