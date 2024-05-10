package lib

import (
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// ByteString is a byte array that serializes to hex
type ByteString []byte

func NewByteStringFromHex(s string) ByteString {
	b, _ := hex.DecodeString(s)
	return ByteString(b)
}

// MarshalJSON serializes ByteArray to hex
func (s ByteString) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON deserializes ByteArray to hex
func (s *ByteString) UnmarshalJSON(data []byte) error {
	var x string
	err := json.Unmarshal(data, &x)
	if err != nil {
		return err
	}
	if len(x) > 0 {
		str, err := hex.DecodeString(x)
		if err != nil {
			return err
		}
		*s = ByteString(str)
	} else {
		*s = nil
	}
	return nil
}

func (s *ByteString) String() string {
	return hex.EncodeToString(*s)
}

func (s ByteString) Value() (driver.Value, error) {
	return []byte(s), nil
}

func (s *ByteString) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	*s = b
	return nil
}
