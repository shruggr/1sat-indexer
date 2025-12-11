package idx

import (
	"encoding/json"
	"fmt"

	"github.com/shruggr/1sat-indexer/v5/lib"
)

// Event represents an indexing event with an ID and value
type Event struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

// EventKey generates a sorted set key for an event
func EventKey(tag string, event *Event) string {
	return fmt.Sprintf("%s:%s:%s", tag, event.Id, event.Value)
}

type IndexData struct {
	Data   any
	Events []*Event
	Deps   []*lib.Outpoint
}

func (id IndexData) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.Data)
}

func (id *IndexData) UnmarshalJSON(data []byte) error {
	id.Data = json.RawMessage([]byte{})
	return json.Unmarshal(data, &id.Data)
}
