package lib

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type File struct {
	Hash     []byte `json:"hash"`
	Size     uint32 `json:"size"`
	Type     string `json:"type"`
	Content  []byte `json:"-"`
	Encoding string `json:"encoding,omitempty"`
	Name     string `json:"name,omitempty"`
}

func (f File) Value() (driver.Value, error) {
	return json.Marshal(f)
}

func (f *File) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &f)
}
