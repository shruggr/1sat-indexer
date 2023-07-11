package lib

import (
	"encoding/json"
)

type Inscription struct {
	JsonContent json.RawMessage `json:"json_content"`
	TextContent string          `json:"text_content"`
	File        *File           `json:"file,omitempty"`
}
