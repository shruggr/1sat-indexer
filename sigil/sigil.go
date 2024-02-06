package sigil

import (
	"encoding/json"
	"log"

	"github.com/libsv/go-bt/bscript"
	"github.com/shruggr/1sat-indexer/lib"
)

func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64, dryRun bool) (ctx *lib.IndexContext) {
	ctx, err := lib.ParseTxn(rawtx, blockId, height, idx)
	if err != nil {
		log.Panicln(err)
	}
	ParseSigil(ctx)
	for _, txo := range ctx.Txos {
		txo.Save()
	}
	return
}

func ParseSigil(ctx *lib.IndexContext) {
	for _, txo := range ctx.Txos {
		if len(txo.PKHash) != 0 {
			continue
		}
		sigil := ParseScript(txo)
		if sigil != nil {
			txo.AddData("sigil", sigil)
		}
	}
}

func ParseScript(txo *lib.Txo) (sigil *json.RawMessage) {
	script := *txo.Tx.Outputs[txo.Outpoint.Vout()].LockingScript
	if len(script) > 49 &&
		script[0] == bscript.OpHASH160 &&
		script[22] == bscript.OpEQUALVERIFY &&
		script[23] == bscript.OpDUP &&
		script[24] == bscript.OpHASH160 &&
		script[46] == bscript.OpEQUALVERIFY &&
		script[47] == bscript.OpCHECKSIG &&
		script[48] == bscript.OpRETURN {

		if txo.Data == nil {
			txo.PKHash = script[26:46]
		}
		pos := 49
		op, err := lib.ReadOp(script, &pos)
		if err == nil {
			var s json.RawMessage
			err = json.Unmarshal(op.Data, &s)
			if err == nil {
				sigil = &s
			}
		}
	}
	return
}
