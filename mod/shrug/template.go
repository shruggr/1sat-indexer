package shrug

import (
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/script/interpreter"
)

func Lock(shrug Shrug, lockingScript *script.Script) (*script.Script, error) {
	s := &script.Script{}
	s.AppendPushData([]byte(SHRUG_TAG))
	if shrug.Id == nil {
		s.AppendOpcodes(script.OpFALSE)
	} else {
		s.AppendPushData(shrug.Id.Bytes())
	}
	s.AppendOpcodes(script.Op2DROP)
	if shrug.Amount.Cmp(interpreter.Zero) == 0 {
		s.AppendOpcodes(script.OpFALSE)
	} else {
		s.AppendPushData((&interpreter.ScriptNumber{Val: shrug.Amount, AfterGenesis: true}).Bytes())
	}
	s.AppendOpcodes(script.OpDROP)
	s = script.NewFromBytes(append(s.Bytes(), lockingScript.Bytes()...))
	return s, nil
}
