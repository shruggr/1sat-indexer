package shrug

import (
	"math/big"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/script/interpreter"
	"github.com/shruggr/1sat-indexer/v5/evt"
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

func (i *ShrugIndexer) FromBytes(data []byte) (any, error) {
	return ShrugFromBytes(data)
}

func (i *ShrugIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	s := idxCtx.Tx.Outputs[vout].LockingScript

	shrug := parseScript(s)
	if shrug != nil {
		idxData := &idx.IndexData{
			Data: shrug,
		}

		if shrug.Id == nil {
			shrug.Status = Valid
			id := lib.NewOutpointFromHash(idxCtx.Txid, vout)
			idxData.Events = []*evt.Event{
				{
					Id: "deploy",
				}, {
					Id:    shrug.Status.String(),
					Value: id.String(),
				},
			}
			if shrug.Amount.Cmp(interpreter.Zero) == 0 {
				idxData.Events = append(idxData.Events, &evt.Event{
					Id:    "mint",
					Value: id.String(),
				})
			}
		} else {
			idxData.Events = []*evt.Event{
				{
					Id:    shrug.Status.String(),
					Value: shrug.Id.String(),
				},
			}

		}
		return idxData
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
	outputs []*idx.IndexData
	deps    []*lib.Outpoint
}

func (i *ShrugIndexer) PreSave(idxCtx *idx.IndexContext) {
	tokens := make(map[string]*shrugToken)

	for _, txo := range idxCtx.Txos {
		if idxData, ok := txo.Data[SHRUG_TAG]; ok {
			if shrug, ok := idxData.Data.(*Shrug); ok {
				if shrug.Id == nil {
					continue
				}
				id := shrug.Id.String()
				if token, ok := tokens[id]; !ok {
					token = &shrugToken{
						outputs: []*idx.IndexData{
							idxData,
						},
					}
					tokens[id] = token
				} else {
					token.outputs = append(token.outputs, idxData)
				}
			}
		}
	}
	if len(tokens) == 0 {
		return
	}

	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == nil {
			// inputs unknown. Leave pending
			return
		}
		if idxData, ok := spend.Data[SHRUG_TAG]; ok {
			if shrug, ok := idxData.Data.(*Shrug); ok {
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
						token.deps = append(token.deps, spend.Outpoint)
					}
				}
			}
		}
	}

	for _, token := range tokens {
		for _, idxData := range token.outputs {
			if shrug, ok := idxData.Data.(*Shrug); ok {
				if !token.hasMint {
					if shrug.Amount.Cmp(interpreter.Zero) == 0 {
						shrug.Status = Invalid
					} else {
						token.balance.Sub(token.balance, shrug.Amount)
					}
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
		for _, idxData := range token.outputs {
			if shrug, ok := idxData.Data.(*Shrug); ok {
				shrug.Status = token.status
				idxData.Events = []*evt.Event{
					{
						Id:    shrug.Status.String(),
						Value: tokenId,
					},
				}
				if shrug.Amount.Cmp(interpreter.Zero) == 0 && token.status == Valid {
					idxData.Events = append(idxData.Events, &evt.Event{
						Id:    "mint",
						Value: shrug.Id.String(),
					})
				}
				idxData.Deps = token.deps
			}
		}
	}
}
