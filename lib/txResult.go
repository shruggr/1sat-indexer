package lib

import "encoding/json"

type OutputMap map[uint32]struct{}

func (oMap OutputMap) MarshalJSON() (bytes []byte, err error) {
	outs := make([]uint32, 0, len(oMap))
	for out := range oMap {
		outs = append(outs, out)
	}
	return json.Marshal(outs)
}

func NewOutputMap() OutputMap {
	oMap := make(OutputMap, 10)
	return oMap
}

type TxResult struct {
	Txid    string    `json:"txid"`
	Outputs OutputMap `json:"outs"`
	Height  uint32    `json:"height"`
	Idx     uint64    `json:"idx"`
	Score   float64   `json:"score"`
}
