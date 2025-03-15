package idx

import (
	"encoding/json"

	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type IndexData struct {
	Data   any             `json:"data,omitempty"`
	Events []*evt.Event    `json:"events,omitempty"`
	Deps   []*lib.Outpoint `json:"deps,omitempty"`
}

func (id IndexData) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.Data)
}

func (id *IndexData) UnmarshalJSON(data []byte) error {
	id.Data = json.RawMessage([]byte{})
	return json.Unmarshal(data, &id.Data)
}
