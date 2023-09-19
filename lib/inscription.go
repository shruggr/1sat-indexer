package lib

import (
	"encoding/json"
)

type Inscription struct {
	Json  json.RawMessage `json:"json,omitempty"`
	Text  string          `json:"text,omitempty"`
	Words []string        `json:"words,omitempty"`
	File  *File           `json:"file,omitempty"`
}
