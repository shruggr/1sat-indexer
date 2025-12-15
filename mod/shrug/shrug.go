package shrug

import (
	"math/big"

	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/script/interpreter"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const SHRUG_TAG = "¯\\_(ツ)_/¯"

type ShrugIndexer struct {
	idx.BaseIndexer
}

func (i *ShrugIndexer) Tag() string {
	return SHRUG_TAG
}

func (i *ShrugIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) any {
	output := idxCtx.Outputs[vout]
	s := idxCtx.Tx.Outputs[vout].LockingScript

	shrug := parseScript(s)
	if shrug != nil {
		if shrug.Id == nil {
			shrug.Status = Valid
			id := lib.NewOutpointFromHash(idxCtx.Txid, vout)
			output.AddEvent(SHRUG_TAG + ":deploy")
			output.AddEvent(SHRUG_TAG + ":" + shrug.Status.String() + ":" + id.String())
			if shrug.Amount.Cmp(interpreter.Zero) == 0 {
				output.AddEvent(SHRUG_TAG + ":mint:" + id.String())
			}
		} else {
			output.AddEvent(SHRUG_TAG + ":" + shrug.Status.String() + ":" + shrug.Id.String())
		}
		return shrug
	}

	return nil
}

func parseScript(s *script.Script) *Shrug {
	shrug := &Shrug{}
	pos := 0
	if op, err := s.ReadOp(&pos); err != nil {
		return nil
	} else if len(op.Data) != 9 || string(op.Data[:8]) != SHRUG_TAG {
		return nil
	} else if op, err = s.ReadOp(&pos); err != nil {
		return nil
	} else if len(op.Data) == 36 {
		shrug.Id = lib.NewOutpointFromBytes(op.Data)
	} else if op, err = s.ReadOp(&pos); err != nil {
		return nil
	} else if op.Op != script.Op2DROP {
		return nil
	} else if op, err = s.ReadOp(&pos); err != nil {
		return nil
	} else if number, err := interpreter.MakeScriptNumber(op.Data, len(op.Data), true, true); err != nil {
		return nil
	} else {
		shrug.Amount = number.Val
	}

	if op, err := s.ReadOp(&pos); err != nil {
		return nil
	} else if op.Op != script.OpDROP {
		return nil
	}
	return shrug
}

type shrugToken struct {
	hasMint bool
	balance *big.Int
	status  ShrugStatus
	outputs []*idx.IndexedOutput
	shrugs  []*Shrug
}

func (i *ShrugIndexer) PreSave(idxCtx *idx.IndexContext) {
	tokens := make(map[string]*shrugToken)

	for _, output := range idxCtx.Outputs {
		if data, ok := output.Data[SHRUG_TAG]; ok {
			if shrug, ok := data.(*Shrug); ok {
				if shrug.Id == nil {
					continue
				}
				id := shrug.Id.String()
				if token, ok := tokens[id]; !ok {
					token = &shrugToken{
						outputs: []*idx.IndexedOutput{output},
						shrugs:  []*Shrug{shrug},
						balance: big.NewInt(0),
					}
					tokens[id] = token
				} else {
					token.outputs = append(token.outputs, output)
					token.shrugs = append(token.shrugs, shrug)
				}
			}
		}
	}
	if len(tokens) == 0 {
		return
	}

	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == 0 {
			// inputs unknown. Leave pending
			return
		}
		if data, ok := spend.Data[SHRUG_TAG]; ok {
			if shrug, ok := data.(*Shrug); ok {
				if shrug.Status == Pending {
					return
				} else if shrug.Status == Valid {
					id := shrug.Id.String()
					if token, ok := tokens[id]; ok {
						if shrug.Amount.Cmp(interpreter.Zero) == 0 {
							token.hasMint = true
						} else {
							token.balance.Add(token.balance, shrug.Amount)
						}
					}
				}
			}
		}
	}

	for _, token := range tokens {
		for _, shrug := range token.shrugs {
			if !token.hasMint {
				if shrug.Amount.Cmp(interpreter.Zero) == 0 {
					shrug.Status = Invalid
				} else {
					token.balance.Sub(token.balance, shrug.Amount)
				}
			}
		}
	}

	for tokenId, token := range tokens {
		if token.status == Pending {
			if token.balance.Cmp(interpreter.Zero) < 0 {
				token.status = Invalid
			} else {
				token.status = Valid
			}
		}
		for j, output := range token.outputs {
			shrug := token.shrugs[j]
			shrug.Status = token.status
			// Clear old events and add new ones
			output.Events = nil
			output.AddEvent(SHRUG_TAG + ":" + shrug.Status.String() + ":" + tokenId)
			if shrug.Amount.Cmp(interpreter.Zero) == 0 && token.status == Valid {
				output.AddEvent(SHRUG_TAG + ":mint:" + shrug.Id.String())
			}
		}
	}
}
