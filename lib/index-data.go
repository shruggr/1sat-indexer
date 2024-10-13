package lib

import "encoding/json"

type Event struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

type IndexData struct {
	Data   any
	Events []*Event
	Deps   []*Outpoint
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
