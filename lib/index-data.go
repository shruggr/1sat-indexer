package lib

import (
	"encoding/json"
)

type Event struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

type IndexData struct {
	Data     any         `json:"-" msgpack:"-"`
	Events   []*Event    `json:"events"`
	FullText string      `json:"text"`
	Deps     []*Outpoint `json:"deps"`
}

// func (id IndexData) MarshalMsgpack() ([]byte, error) {
// 	if data, err := json.Marshal(id.Data); err != nil {
// 		return nil, err
// 	} else {
// 		return msgpack.Marshal(data)
// 	}
// }

// func (id *IndexData) UnmarshalMsgpack(data []byte) error {
// 	var unmarshalled json.RawMessage
// 	if err := msgpack.Unmarshal(data, &unmarshalled); err != nil {
// 		return err
// 	}
// 	id.Data = unmarshalled
// 	return nil
// }

func (id IndexData) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.Data)
}

func (id *IndexData) UnmarshalJSON(data []byte) error {
	id.Data = json.RawMessage([]byte{})
	return json.Unmarshal(data, &id.Data)
}

// func (id IndexData) MarshalBinary() ([]byte, error) {
// 	return json.Marshal(id.Data)
// }

// func (id *IndexData) UnmarshalBinary(data []byte) error {
// 	id.Data = json.RawMessage([]byte{})
// 	return json.Unmarshal(data, &id.Data)
// }
