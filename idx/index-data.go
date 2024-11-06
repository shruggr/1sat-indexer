package idx

import (
	"encoding/json"

	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

type IndexData struct {
	Data   any
	Events []*evt.Event
	Deps   []*lib.Outpoint
	// DepQueue    []*Outpoint `json:"-" msgpack:"-"`
	PostProcess bool `json:"-" msgpack:"-"`
	// FullText    string      `json:"text,omitempty"`
}

func (id IndexData) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.Data)
}

func (id *IndexData) UnmarshalJSON(data []byte) error {
	id.Data = json.RawMessage([]byte{})
	return json.Unmarshal(data, &id.Data)
}
